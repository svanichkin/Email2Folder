package email

import (
	"bytes"
	"testing"

	"github.com/emersion/go-imap/v2"
)

func TestWholeMessageBodyAcceptsServerResponseToBodyPeek(t *testing.T) {
	// IMAP responds to BODY.PEEK[] as BODY[]; therefore the response section
	// does not retain the request's Peek flag or pointer identity.
	responseSection := &imap.FetchItemBodySection{}
	want := []byte("Subject: example\r\n\r\nmessage\r\n")
	got := wholeMessageBody(map[*imap.FetchItemBodySection][]byte{responseSection: want})
	if !bytes.Equal(got, want) {
		t.Fatalf("wholeMessageBody() = %q, want original MIME bytes", got)
	}
}

func TestDefaultMailboxNameUsesPersonalNamespacePrefix(t *testing.T) {
	client := &IMAPClient{personalPrefix: "INBOX."}
	if got, want := client.DefaultMailboxName("Email2Folder Processing"), "INBOX.Email2Folder Processing"; got != want {
		t.Fatalf("DefaultMailboxName() = %q, want %q", got, want)
	}
}

func TestDefaultMailboxNameFallsBackToRoot(t *testing.T) {
	client := &IMAPClient{}
	if got, want := client.DefaultMailboxName("Email2Folder Processing"), "Email2Folder Processing"; got != want {
		t.Fatalf("DefaultMailboxName() = %q, want %q", got, want)
	}
}
