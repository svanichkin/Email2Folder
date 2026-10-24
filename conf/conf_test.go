package conf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoteAITokenValueFromKeysDirectory(t *testing.T) {
	keysDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(keysDir, "remoteai"), []byte(" token-from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}

	token, err := (Config{Keys: keysDir, RemoteAIToken: "remoteai"}).RemoteAITokenValue()
	if err != nil {
		t.Fatal(err)
	}
	if token != "token-from-file" {
		t.Fatalf("token = %q, want %q", token, "token-from-file")
	}
}

func TestRemoteAITokenValueKeepsInlineTokenWithoutKeysDirectory(t *testing.T) {
	token, err := (Config{RemoteAIToken: "inline-token"}).RemoteAITokenValue()
	if err != nil {
		t.Fatal(err)
	}
	if token != "inline-token" {
		t.Fatalf("token = %q, want %q", token, "inline-token")
	}
}

func TestRemoteAITokenValueReportsMissingKeyFile(t *testing.T) {
	_, err := (Config{Keys: t.TempDir(), RemoteAIToken: "remoteai"}).RemoteAITokenValue()
	if err == nil {
		t.Fatal("expected an error for a missing key file")
	}
}
