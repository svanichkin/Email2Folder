package remoteai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnalyzeRawEmailUsesOneDeepSeekRequestWithWholeSource(t *testing.T) {
	raw := []byte("From: sender@example.com\r\nSubject: caf\xe9\r\n\r\nBody")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var request chatCompletionRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != deepSeekModel {
			t.Errorf("model = %q, want %q", request.Model, deepSeekModel)
		}
		if len(request.Messages) != 1 || !strings.Contains(request.Messages[0].Content, rawEmailState(raw)) {
			t.Error("prompt does not contain complete raw email source")
		}
		if request.Reasoning.Enabled {
			t.Error("reasoning must be disabled to reserve completion for JSON")
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"type\":\"tracking\",\"markdown\":\"# Доставка\",\"summary\":\"Посылка прибыла\",\"tags\":\"#доставка\",\"unsubscribe\":\"\"}"}}]}`))
	}))
	defer server.Close()

	client := &RemoteAIClient{httpClient: server.Client(), baseURL: server.URL, token: "test-token"}
	got, err := client.AnalyzeRawEmail(context.Background(), raw)
	if err != nil {
		t.Fatalf("AnalyzeRawEmail: %v", err)
	}
	if requests != 1 {
		t.Errorf("requests = %d, want 1", requests)
	}
	if got.Type != TypeTracking || got.Markdown != "# Доставка" || got.Summary != "Посылка прибыла" || got.Tags != "#доставка" {
		t.Errorf("analysis = %#v", got)
	}
}

func TestAnalyzeRawEmailRejectsIncompleteResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"type\":\"notification\",\"markdown\":\"\"}"}}]}`))
	}))
	defer server.Close()

	client := &RemoteAIClient{httpClient: server.Client(), baseURL: server.URL, token: "test-token"}
	if _, err := client.AnalyzeRawEmail(context.Background(), []byte("From: a@b\r\n\r\nbody")); err == nil {
		t.Fatal("AnalyzeRawEmail succeeded with empty markdown")
	}
}

func TestRawEmailStatePreservesNonUTF8Bytes(t *testing.T) {
	raw := []byte{0x00, 0x7f, 0x80, 0xff}
	runes := []rune(rawEmailState(raw))
	for index, want := range raw {
		if got := runes[index]; got != rune(want) {
			t.Fatalf("byte %d = %x, want %x", index, got, want)
		}
	}
}
