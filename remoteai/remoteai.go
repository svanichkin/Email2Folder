package remoteai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	au "github.com/logrusorgru/aurora/v4"
	"github.com/sashabaranov/go-openai"
)

type RemoteAIClient struct {
	client       *openai.Client
	systemPrompt string
}

func NewRemoteAIClient(token string) (*RemoteAIClient, error) {

	if token == "" {
		log.Println(au.Gray(12, "[REMOTEAI]").String() + " " + au.Yellow("No RemoteAI token provided, client will be disabled").String())
		return nil, nil
	}
	log.Println(au.Gray(12, "[REMOTEAI]").String() + " " + au.Cyan("Initializing OpenRouter client...").String())
	
	config := openai.DefaultConfig(token)
	config.BaseURL = "https://openrouter.ai/api/v1"
	client := openai.NewClientWithConfig(config)

	systemPrompt :=
		`
Проанализируй письмо и верни результат в формате JSON. Не добавляй ничего от себя.

Если уверенность 95% или выше, выбери один из типов:
- spam — спам
- phishing — фишинг
- notification — уведомление от банка/сервиса
- code — код входа/подтверждения
- human — личная переписка
- unknown — если не подходит ни к одному типу

Формат:
{
  "type": "spam|phishing|notification|code|human|unknown",
  "language": "Определи язык на котором письмо написано",
  "summary": "Краткое описание письма на русском языке",
  "tags": "Напиши хештеги",
  "unsubscribe": "URL для отписки, если есть" // поле необязательное
}
Если type = "code", в summary укажи только сам код.
`
	log.Println(au.Gray(12, "[REMOTEAI]").String() + " " + au.Green(au.Bold("RemoteAI client initialized successfully")).String())
	return &RemoteAIClient{
		client:       client,
		systemPrompt: systemPrompt,
	}, nil
}

type EmailType string

const (
	TypeSpam         EmailType = "spam"
	TypePhishing     EmailType = "phishing"
	TypeNotification EmailType = "notification"
	TypeCode         EmailType = "code"
	TypeHuman        EmailType = "human"
	TypeUnknown      EmailType = "unknown"
)

type EmailAnalysisResult struct {
	Type        EmailType `json:"type"`
	Summary     string    `json:"summary"`
	Unsubscribe string    `json:"unsubscribe,omitempty"`
	Tags        string    `json:"tags"`
}

func (oac *RemoteAIClient) GenerateTextFromEmail(emailText string) (*EmailAnalysisResult, error) {

	if oac.client == nil {
		return nil, errors.New("RemoteAI client not initialized")
	}
	if emailText == "" {
		return nil, errors.New("email text is empty")
	}

	log.Println(au.Gray(12, "[REMOTEAI]").String() + " " + au.Magenta("Analyzing email content with OpenRouter...").String())
	resp, err := oac.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: "google/gemini-3.1-flash-lite-preview",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: oac.systemPrompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: emailText,
				},
			},
			Temperature: 0.25,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("RemoteAI chat completion error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New("RemoteAI returned no choices")
	}

	log.Println(au.Gray(12, "[REMOTEAI]").String() + " " + au.Blue("Received response from RemoteAI, processing...").String())
	content := cleanRemoteAIResponse(resp.Choices[0].Message.Content)

	var result EmailAnalysisResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse RemoteAI response as JSON: %w\nResponse: %s", err, content)
	}

	log.Printf(au.Gray(12, "[REMOTEAI]").String()+" "+au.Green("Analysis completed. Type: %s, Unsubscribe: %t, Summary: %t").String(), string(result.Type), result.Summary != "", result.Unsubscribe != "")
	return &result, nil
}

func cleanRemoteAIResponse(resp string) string {

	log.Println(au.Gray(12, "[REMOTEAI]").String() + " " + au.Cyan("Cleaning RemoteAI response...").String())
	re := regexp.MustCompile("(?s)^```json\\s*(.*)\\s*```$|^```\\s*(.*)\\s*```$")
	matches := re.FindStringSubmatch(resp)
	if len(matches) > 0 {
		for _, m := range matches[1:] {
			if m != "" {
				return m
			}
		}
	}

	return strings.TrimSpace(resp)
}
