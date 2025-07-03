package main

import (
	"email2folder/conf"
	"email2folder/email"
	"email2folder/file"

	"bytes"
	"fmt"
	"log"

	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
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

	config, err := conf.Init()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	emailAddresses, err := file.FindEmailAddresses(config.EmailAdresses)
	if err != nil {
		log.Fatalf("Failed to initialize email files: %v", err)
	}

	passwordFiles, err := file.FindPasswordFiles(config.Passwords, getKeys(emailAddresses))
	if err != nil {
		log.Fatalf("Failed to initialize password files: %v", err)
	}

	// Пока работаем жестко лишь с одним сервером

	p := passwordFiles["regru"]
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

	password = p[username]

	folder = config.Folder

	for {
		now := time.Now()
		nextRun := now.Truncate(time.Minute).Add(time.Minute).Add(scheduleTime)
		sleepDuration := nextRun.Sub(now)

		log.Printf("Next run at %s (in %v)", nextRun.Format("15:04:05"), sleepDuration.Round(time.Second))
		time.Sleep(sleepDuration)

		processEmails()
	}

}

func processEmails() {

	log.Println("Connecting to POP3 server...")
	startTime := time.Now()

	// Инициализация POP3 клиента

	c, err := email.InitPop3(serverAddr, username, password, timeout)
	if err != nil {
		log.Println("Pop3 connection error:", err)
		return
	}
	defer c.Quit()

	// Получение статистики

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

	// Получение списка писем

	msgs, err := c.List(0)
	if err != nil {
		log.Println("List error:", err)
		return
	}

	// Обработка писем

	for _, msg := range msgs {

		// Проверка времени выполнения

		if time.Since(startTime) > timeout {
			log.Println("Timeout reached, stopping processing")
			break
		}

		// Получение письма

		m, err := c.Retr(msg.ID)
		if err != nil {
			log.Printf("Retrieve error for message %d: %v", msg.ID, err)
			continue
		}

		// Чтение письма

		var buf bytes.Buffer
		if err := m.WriteTo(&buf); err != nil {
			log.Printf("Read error for message %d: %v", msg.ID, err)
			continue
		}
		msgData := buf.Bytes()

		// Парсинг заголовков

		header, err := email.ExtractHeader(msgData)
		if err != nil {
			log.Printf("Parse headers error: %v", err)
			continue
		}

		// Поиск или создание папки

		fromAddresses := email.ExtractAddresses(header, "From")
		folderPath, err := findOrCreateFolder(fromAddresses)
		if err != nil {
			log.Printf("Folder error: %v", err)
			continue
		}

		// Создание безопасного имени файла

		s, _ := header.Subject()
		filename := fmt.Sprintf("%s %s.eml", email.ExtractDate(header).Format("2006-01-02 15꞉04"), file.CleanFileName(s))

		// Папка уже создана в findOrCreateFolder

		filePath := filepath.Join(folderPath, filename)

		// Сохранение письма

		if err := os.WriteFile(filePath, msgData, 0644); err != nil {
			log.Printf("Write file error: %v", err)
			continue
		}

		// Установка xattr для файла

		if err := file.SetAttributes(filePath, initFileAttributes(header, m)); err != nil {
			log.Printf("Set attributes error: %v", err)
		}

		// Удаление письма с сервера

		if err := c.Dele(msg.ID); err != nil {
			log.Printf("Delete error: %v", err)
		}

		log.Printf("Processed message %d from %s", msg.ID, fromAddresses[0])
	}

}

func findOrCreateFolder(fromAddresses []string) (string, error) {

	fromKey := strings.Join(fromAddresses, ",")

	// 1. Поиск существующей папки с нужным атрибутом

	if foundPath, err := file.FindFoldeArttrFrom(folder, fromKey); err == nil {
		return foundPath, nil
	}

	// 2. Если не найдено — создаем новую папку

	return file.CreateNewFolder(filepath.Join(folder, file.CleanFolderName(fromAddresses[0])), map[string]string{"from": fromKey})

}

// СОЗДАНИЕ НОВОЙ ПАПКИ

func getKeys(m map[string]string) []string {

	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	return keys

}

func initFileAttributes(header mail.Header, msg *message.Entity) map[string]string {

	return map[string]string{
		"from":        strings.Join(email.ExtractAddresses(header, "From"), ","),
		"to":          strings.Join(email.ExtractAddresses(header, "To"), ","),
		"date":        fmt.Sprintf("%d", email.ExtractDate(header).Unix()),
		"attachments": strconv.FormatBool(email.HasAttachments(msg)),
		"status":      "unseen",
	}

}
