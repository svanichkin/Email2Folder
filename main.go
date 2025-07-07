package main

import (
	"email2folder/conf"
	"email2folder/email"
	"email2folder/file"
	"email2folder/openai"
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
	au "github.com/logrusorgru/aurora/v4"
)

const (
	timeout      = 10 * time.Second
	scheduleTime = 0 * time.Second
)

var serverAddr string
var username string
var password string
var folder string

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

	// Init OpenAI

	var ai *openai.OpenAIClient
	ai, err = openai.NewOpenAIClient(config.OpenAIToken)
	if err != nil {
		log.Fatalf(au.Gray(12, "[INIT]").String()+" "+au.Yellow(au.Bold("Warning: Failed to initialize OpenAI client: %v. OpenAI features will be disabled.")).String(), err)
	} else if ai == nil {
		log.Fatalf(au.Gray(12, "[INIT]").String() + " " + au.Yellow("OpenAI token not provided or empty. OpenAI features will be disabled.").String())
	} else {
		log.Println(au.Gray(12, "[INIT]").String() + " " + au.Green("OpenAI client initialized successfully.").String())
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

func processEmails(ai *openai.OpenAIClient) {

	log.Println("Connecting to POP3 server...")
	startTime := time.Now()

	// Init POP3 client

	c, err := email.InitPop3(serverAddr, username, password, timeout)
	if err != nil {
		log.Println("Pop3 connection error:", err)
		return
	}
	defer c.Quit()

	// Get statistics

	count, _, err := c.Stat()
	if err != nil {
		log.Println("Stat error:", err)
		return
	}
	if count == 0 {
		log.Println("No messages found")
		return
	}
	log.Printf("Found %d messages, processing...", count)

	// Get emails list

	msgs, err := c.List(0)
	if err != nil {
		log.Println("List error:", err)
		return
	}

	// Work with emails

	for _, msg := range msgs {

		// Check timout

		if time.Since(startTime) > timeout {
			log.Println("Timeout reached, stopping processing")
			break
		}

		// Get next mail

		m, err := c.Retr(msg.ID)
		if err != nil {
			log.Printf("Retrieve error for message %d: %v", msg.ID, err)
			continue
		}
		var buf bytes.Buffer
		if err := m.WriteTo(&buf); err != nil {
			log.Printf("Read error for message %d: %v", msg.ID, err)
			continue
		}
		msgData := buf.Bytes()

		// Parse headers

		header, err := email.ExtractHeader(msgData)
		if err != nil {
			log.Printf("Parse headers error: %v", err)
			continue
		}

		// Get attributes

		attrs, err := initFileAttributes(header, msgData)
		if err != nil {
			log.Printf("Attributes error: %v", err)
			continue
		}

		// Openai attributwes

		res, err := ai.GenerateTextFromEmail(attrs["markdown"])
		if err != nil {
			log.Printf("Openai attributes error: %v", err)
			continue
		}
		attrs["type"] = string(res.Type)
		attrs["summary"] = string(res.Summary)
		if len(attrs["unsubscribe"]) == 0 {
			attrs["unsubscribe"] = string(res.Unsubscribe)
		}
		attrs["tags"] = string(res.Tags)

		// Find and create folder

		fromAddresses := email.ExtractAddresses(header, "From")
		folderPath, err := findOrCreateFolderAttrFrom(fromAddresses)
		if err != nil {
			log.Printf("Folder error: %v", err)
			continue
		}

		// Safity filename

		s, _ := header.Subject()
		filename := file.TrimFilenameToMaxBytes(fmt.Sprintf("%s %s.eml", email.ExtractDate(header).Format("2006-01-02 15꞉04"), file.CleanFileName(s)), 254)

		// Folder created in findOrCreateFolder

		filePath := filepath.Join(folderPath, filename)

		// Create .eml

		if err := os.WriteFile(filePath, msgData, 0644); err != nil {
			log.Printf("Write file error: %v", err)
			continue
		}

		// Set xattr for .eml

		if err := file.SetAttributes(filePath, attrs); err != nil {
			log.Printf("Set attributes error: %v", err)
		}

		// Delete email from server

		if err := c.Dele(msg.ID); err != nil {
			log.Printf("Delete error: %v", err)
		}
		log.Printf("Processed message %d from %s", msg.ID, fromAddresses[0])
	}

}

func findOrCreateFolderAttrFrom(fromAddresses []string) (string, error) {

	fromKey := strings.Join(fromAddresses, ",")

	// Find folder wtih xattr from

	if foundPath, err := file.FindFoldeArttrFrom(folder, fromKey); err == nil {
		return foundPath, nil
	}

	// If not found - create new folder

	return file.CreateNewFolder(filepath.Join(folder, file.CleanFolderName(fromAddresses[0])), map[string]string{"from": fromKey})

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
		"from":        strings.Join(email.ExtractAddresses(header, "From"), ","),
		"to":          strings.Join(email.ExtractAddresses(header, "To"), ","),
		"markdown":    md,
		"date":        fmt.Sprintf("%d", email.ExtractDate(header).Unix()),
		"attachments": strconv.FormatBool(email.HasAttachments(data)),
		"status":      "unseen",
		"unsubscribe": strings.Join(email.ExtractUnsubscribe(header), ","),
	}, nil

}
