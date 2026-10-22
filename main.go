package main

import (
	"context"
	"email2folder/conf"
	"email2folder/email"
	"email2folder/file"
	"email2folder/remoteai"
	"os/exec"

	"bytes"
	"fmt"
	"log"

	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"
	"github.com/jhillyerd/enmime"
	au "github.com/logrusorgru/aurora/v4"
)

const (
	timeout                      = 10 * time.Second
	imapInbox                    = "INBOX"
	defaultIMAPProcessingMailbox = "Email2Folder Processing"
)

var serverAddr string
var username string
var password string
var folder string

var scheduleTime time.Duration
var imapProcessingMailbox string

func main() {

	// Config parsing

	config, err := conf.Init()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	// Start cycle every config.StartTimeSecond one time in minute

	scheduleTime = config.StartTimeSecond * time.Second

	// Find all email addresses

	emailAddresses, err := file.FindEmailAddresses(config.Addresses)
	if err != nil {
		log.Fatalf("Failed to initialize email files: %v", err)
	}

	// Find all passwords for email addresses

	passwordFiles, err := file.FindPasswordFiles(config.Passwords, getKeys(emailAddresses))
	if err != nil {
		log.Fatalf("Failed to initialize password files: %v", err)
	}

	// Only one server limit

	var emailAddress string
	for emailAddress = range emailAddresses {
		break
	}
	p := passwordFiles[emailAddress]
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		log.Fatalf("Failed to initialize username: %v", err)
	}
	username = keys[0]
	parts := strings.Split(username, "@")
	if len(parts) == 2 {
		serverAddr = parts[1]
	} else {
		log.Fatalf("Failed to initialize server address: %v", err)
	}

	// Init parameters

	password = p[username]
	folder = config.Folder
	if config.IMAPServer != "" {
		serverAddr = config.IMAPServer
	}
	imapProcessingMailbox = config.IMAPProcessingMailbox
	if imapProcessingMailbox == "" {
		imapProcessingMailbox = defaultIMAPProcessingMailbox
	}

	// Init RemoteAI

	var ai *remoteai.RemoteAIClient
	ai, err = remoteai.NewRemoteAIClient(config.RemoteAIToken)
	if err != nil {
		log.Fatalf(au.Gray(12, "[INIT]").String()+" "+au.Yellow(au.Bold("Warning: Failed to initialize RemoteAI client: %v. RemoteAI features will be disabled.")).String(), err)
	} else if ai == nil {
		log.Fatalf(au.Gray(12, "[INIT]").String() + " " + au.Yellow("RemoteAI token not provided or empty. RemoteAI features will be disabled.").String())
	} else {
		log.Println(au.Gray(12, "[INIT]").String() + " " + au.Green("RemoteAI client initialized successfully.").String())
	}

	// Check if service updated with last modify time

	exePath, _ := os.Executable()
	info, _ := os.Stat(exePath)
	lastModTime := info.ModTime()

	// Main cycle

	for {
		now := time.Now()
		nextRun := now.Truncate(time.Minute).Add(time.Minute).Add(scheduleTime)
		sleepDuration := nextRun.Sub(now)
		log.Printf("Next run at %s (in %v)", nextRun.Format("15:04:05"), sleepDuration.Round(time.Second))
		time.Sleep(sleepDuration)
		processEmails(ai)
		info, err := os.Stat(exePath)
		if err != nil {
			continue
		}
		if info.ModTime() != lastModTime {
			fmt.Println("Binary updated, stop service...")
			exec.Command(exePath).Start()
			os.Exit(0)
		}
	}

}

func processEmails(ai *remoteai.RemoteAIClient) {

	log.Println("Connecting to IMAP server...")
	startTime := time.Now()

	// Init IMAP client

	c, err := email.InitIMAP(serverAddr, username, password, timeout)
	if err != nil {
		log.Println("IMAP connection error:", err)
		return
	}
	defer func() {
		if err := c.Close(); err != nil {
			log.Printf("IMAP close error: %v", err)
		}
	}()

	if err := c.EnsureMailbox(imapProcessingMailbox); err != nil {
		log.Printf("Processing mailbox error: %v", err)
		return
	}

	processed := 0
	for {

		// Check timout

		if time.Since(startTime) > timeout {
			log.Println("Timeout reached, stopping processing")
			break
		}

		// Atomically claim the next message by moving it out of INBOX.

		claim, err := c.ClaimNext(imapInbox, imapProcessingMailbox)
		if err != nil {
			log.Printf("Claim message error: %v", err)
			break
		}
		if claim == nil {
			break
		}

		msgData, err := c.FetchClaimedRaw(imapProcessingMailbox, claim.UID)
		if err != nil {
			log.Printf("Retrieve error for message %d: %v", claim.UID, err)
			releaseClaim(c, claim)
			break
		}

		// Parse headers

		header, err := email.ExtractHeader(msgData)
		if err != nil {
			log.Printf("Parse headers error: %v", err)
			releaseClaim(c, claim)
			break
		}

		// Get attributes

		attrs, err := initFileAttributes(header, msgData)
		if err != nil {
			log.Printf("Attributes error: %v", err)
			releaseClaim(c, claim)
			break
		}

		// Openai attributwes

		remaining := timeout - time.Since(startTime)
		if remaining <= 0 {
			log.Println("Timeout reached, stopping processing")
			releaseClaim(c, claim)
			break
		}
		aiCtx, cancel := context.WithTimeout(context.Background(), remaining)
		res, err := ai.GenerateTextFromEmail(aiCtx, attrs["markdown"])
		cancel()
		if err != nil {
			log.Printf("Openai attributes error: %v", err)
			releaseClaim(c, claim)
			break
		}
		attrs["type"] = string(res.Type)
		attrs["summary"] = string(res.Summary)
		if len(attrs["unsubscribe"]) == 0 {
			attrs["unsubscribe"] = string(res.Unsubscribe)
		}
		attrs["tags"] = string(res.Tags)

		// Find and create folder

		fromAddresses := email.ExtractFrom(header)
		folderPath, err := findOrCreateFolderAttrFrom(fromAddresses)
		if err != nil {
			log.Printf("Folder error: %v", err)
			releaseClaim(c, claim)
			break
		}

		if err := saveMessage(folderPath, header, msgData, attrs); err != nil {
			log.Printf("Save message error: %v", err)
			releaseClaim(c, claim)
			break
		}

		// Delete only after the complete message was stored successfully.

		if err := c.DeleteClaimed(claim.UID); err != nil {
			log.Printf("Delete error for message %d: %v", claim.UID, err)
			releaseClaim(c, claim)
			break
		}
		processed++
		log.Printf("Processed message %d from %s", claim.UID, fromAddresses[0])
	}
	if processed == 0 {
		log.Println("No messages processed")
	}

}

func releaseClaim(c *email.IMAPClient, claim *email.ClaimedMessage) {

	if err := c.ReleaseClaim(imapInbox, claim.UID); err != nil {
		log.Printf("Release message %d error: %v", claim.UID, err)
	}

}

func saveMessage(folderPath string, header mail.Header, msgData []byte, attrs map[string]string) error {

	subject, _ := header.Subject()
	date := email.ExtractDate(header).Format("2006-01-02 15꞉04")
	filename := file.TrimFilenameToMaxBytes(fmt.Sprintf("%s %s.eml", date, file.CleanFileName(subject)), 254)
	filePath := filepath.Join(folderPath, filename)
	if err := os.WriteFile(filePath, msgData, 0644); err != nil {
		return fmt.Errorf("write .eml: %w", err)
	}
	if err := file.SetAttributes(filePath, attrs); err != nil {
		return fmt.Errorf("set attributes: %w", err)
	}

	env, err := enmime.ReadEnvelope(bytes.NewReader(msgData))
	if err != nil {
		return fmt.Errorf("parse MIME: %w", err)
	}
	for _, att := range env.Attachments {
		attachmentName := file.CleanAttachmentFileName(att.FileName)
		attachmentPath := filepath.Join(folderPath, file.TrimFilenameToMaxBytes(date+" "+attachmentName, 254))
		if err := os.WriteFile(attachmentPath, att.Content, 0644); err != nil {
			return fmt.Errorf("save attachment %q: %w", attachmentName, err)
		}
	}

	return nil

}

func findOrCreateFolderAttrFrom(fromAddresses []string) (string, error) {

	if len(fromAddresses) == 0 {
		return "", fmt.Errorf("no From address in message")
	}
	fromKey := strings.Join(fromAddresses, ",")

	// Find folder wtih xattr from

	if foundPath, err := file.FindFoldeArttrFrom(folder, fromKey); err == nil {
		return foundPath, nil
	}

	// If not found - create new folder
	from := map[string]string{"from": fromKey}
	folderName := file.CleanFolderName(fromAddresses[0])
	fullPath := filepath.Join(folder, folderName)

	return file.CreateNewFolder(fullPath, from)

}

func getKeys(m map[string]string) []string {

	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	return keys

}

func initFileAttributes(header mail.Header, data []byte) (map[string]string, error) {

	md, err := email.ExtractText(header, data)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"from":        strings.Join(email.ExtractFrom(header), ","),
		"to":          strings.Join(email.ExtractAddresses(header, "To"), ","),
		"markdown":    md,
		"date":        fmt.Sprintf("%d", email.ExtractDate(header).Unix()),
		"attachments": strconv.FormatBool(email.HasAttachments(data)),
		"status":      "unseen",
		"unsubscribe": strings.Join(email.ExtractUnsubscribe(header), ","),
	}, nil

}
