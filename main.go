package main

import (
	"context"
	"email2folder/conf"
	"email2folder/email"
	"email2folder/file"
	"email2folder/remoteai"

	"bytes"
	"errors"
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
	connectionTimeout            = 10 * time.Second
	aiRequestTimeout             = 45 * time.Second
	idleRefreshInterval          = 29 * time.Minute
	reconnectDelay               = 5 * time.Second
	imapInbox                    = "INBOX"
	defaultIMAPProcessingMailbox = "Email2Folder Processing"
)

var serverAddr string
var username string
var password string
var folder string

var imapProcessingMailbox string

func main() {

	// Config parsing

	config, err := conf.Init()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

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

	// Init RemoteAI

	var ai *remoteai.RemoteAIClient
	remoteAIToken, err := config.RemoteAITokenValue()
	if err != nil {
		log.Fatalf(au.Gray(12, "[INIT]").String()+" "+au.Yellow("Failed to load RemoteAI token: %v").String(), err)
	}
	ai, err = remoteai.NewRemoteAIClient(remoteAIToken)
	if err != nil {
		log.Fatalf(au.Gray(12, "[INIT]").String()+" "+au.Yellow(au.Bold("Warning: Failed to initialize RemoteAI client: %v. RemoteAI features will be disabled.")).String(), err)
	} else if ai == nil {
		log.Fatalf(au.Gray(12, "[INIT]").String() + " " + au.Yellow("RemoteAI token not provided or empty. RemoteAI features will be disabled.").String())
	} else {
		log.Println(au.Gray(12, "[INIT]").String() + " " + au.Green("RemoteAI client initialized successfully.").String())
	}

	runIMAPIdleLoop(ai)
}

func runIMAPIdleLoop(ai *remoteai.RemoteAIClient) {
	for {
		log.Println("Connecting to IMAP server...")
		c, err := email.InitIMAP(serverAddr, username, password, connectionTimeout)
		if err != nil {
			log.Printf("IMAP connection error: %v; reconnecting in %v", err, reconnectDelay)
			time.Sleep(reconnectDelay)
			continue
		}
		if imapProcessingMailbox == "" {
			imapProcessingMailbox = c.DefaultMailboxName(defaultIMAPProcessingMailbox)
			log.Printf("Using default processing mailbox %q", imapProcessingMailbox)
		}
		if err := c.EnsureMailbox(imapProcessingMailbox); err != nil {
			log.Printf("Processing mailbox error: %v", err)
			_ = c.Close()
			time.Sleep(reconnectDelay)
			continue
		}

		for {
			if err := processEmails(c, ai); err != nil {
				log.Printf("Email processing error: %v", err)
				break
			}

			idleCtx, cancel := context.WithTimeout(context.Background(), idleRefreshInterval)
			err := c.WaitForInboxChange(idleCtx, imapInbox)
			cancel()
			if err == nil {
				log.Println("IMAP IDLE received a mailbox update")
				continue
			}
			if errors.Is(err, context.DeadlineExceeded) {
				log.Println("Refreshing IMAP IDLE connection")
				continue
			}
			log.Printf("IMAP IDLE error: %v", err)
			break
		}

		if err := c.Close(); err != nil {
			log.Printf("IMAP close error: %v", err)
		}
		log.Printf("Reconnecting to IMAP in %v", reconnectDelay)
		time.Sleep(reconnectDelay)
	}
}

func processEmails(c *email.IMAPClient, ai *remoteai.RemoteAIClient) error {

	processed := 0
	for {
		// Atomically claim the next message by moving it out of INBOX.

		claim, err := c.ClaimNext(imapInbox, imapProcessingMailbox)
		if err != nil {
			return fmt.Errorf("claim message: %w", err)
		}
		if claim == nil {
			break
		}

		msgData, err := c.FetchClaimedRaw(imapProcessingMailbox, claim.UID)
		if err != nil {
			releaseClaim(c, claim)
			return fmt.Errorf("retrieve message %d: %w", claim.UID, err)
		}

		// Parse headers

		header, err := email.ExtractHeader(msgData)
		if err != nil {
			releaseClaim(c, claim)
			return fmt.Errorf("parse headers for message %d: %w", claim.UID, err)
		}

		// Get attributes

		attrs, err := initFileAttributes(header, msgData)
		if err != nil {
			releaseClaim(c, claim)
			return fmt.Errorf("build attributes for message %d: %w", claim.UID, err)
		}

		// DeepSeek receives the original RFC 5322/MIME email once and returns
		// its type, Russian Markdown, and all metadata in one JSON response.

		aiCtx, cancel := context.WithTimeout(context.Background(), aiRequestTimeout)
		res, err := ai.AnalyzeRawEmail(aiCtx, msgData)
		cancel()
		if err != nil {
			releaseClaim(c, claim)
			return fmt.Errorf("DeepSeek analysis for message %d: %w", claim.UID, err)
		}
		attrs["type"] = string(res.Type)
		attrs["markdown"] = res.Markdown
		attrs["summary"] = string(res.Summary)
		if len(attrs["unsubscribe"]) == 0 {
			attrs["unsubscribe"] = string(res.Unsubscribe)
		}
		attrs["tags"] = string(res.Tags)

		// Find and create folder

		fromAddresses := email.ExtractFrom(header)
		folderPath, err := findOrCreateFolderAttrFrom(fromAddresses)
		if err != nil {
			releaseClaim(c, claim)
			return fmt.Errorf("folder for message %d: %w", claim.UID, err)
		}

		if err := saveMessage(folderPath, header, msgData, attrs); err != nil {
			releaseClaim(c, claim)
			return fmt.Errorf("save message %d: %w", claim.UID, err)
		}

		// Delete only after the complete message was stored successfully.

		if err := c.DeleteClaimed(claim.UID); err != nil {
			releaseClaim(c, claim)
			return fmt.Errorf("delete message %d: %w", claim.UID, err)
		}
		processed++
		log.Printf("Processed message %d from %s", claim.UID, fromAddresses[0])
	}
	if processed == 0 {
		log.Println("INBOX is empty; waiting in IMAP IDLE")
	}
	return nil
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
