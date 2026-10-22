package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// IMAPClient claims a message by atomically moving it from INBOX to a shared
// processing mailbox. Only the worker that receives the destination UID uses
// that claim, so concurrent workers never process the same message.
type IMAPClient struct {
	client *imapclient.Client
}

// ClaimedMessage identifies a message after it was moved to the processing
// mailbox. The UID is valid only while that mailbox remains selected.
type ClaimedMessage struct {
	UID imap.UID
}

func InitIMAP(serverAddr, username, password string, timeout time.Duration) (*IMAPClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	dialer := &net.Dialer{Timeout: timeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(serverAddr, "993"))
	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(rawConn, &tls.Config{ServerName: serverAddr, MinVersion: tls.VersionTLS12})
	if err := tlsConn.SetDeadline(time.Now().Add(timeout)); err != nil {
		rawConn.Close()
		return nil, err
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, err
	}

	client := imapclient.New(&deadlineConn{Conn: tlsConn, timeout: timeout}, nil)
	if err := client.WaitGreeting(); err != nil {
		client.Close()
		return nil, err
	}
	if err := client.Login(username, password).Wait(); err != nil {
		client.Close()
		return nil, err
	}
	caps := client.Caps()
	if !caps.Has(imap.CapMove) || !caps.Has(imap.CapUIDPlus) {
		client.Close()
		return nil, errors.New("IMAP server must support MOVE and UIDPLUS for safe multi-machine processing")
	}

	return &IMAPClient{client: client}, nil
}

// EnsureMailbox creates a top-level processing mailbox if it doesn't exist.
func (c *IMAPClient) EnsureMailbox(mailbox string) error {
	if _, err := c.client.Select(mailbox, nil).Wait(); err == nil {
		return nil
	}
	if err := c.client.Create(mailbox, nil).Wait(); err != nil {
		return fmt.Errorf("create processing mailbox %q: %w", mailbox, err)
	}
	return nil
}

// ClaimNext moves one message from inbox to processingMailbox. IMAP MOVE is
// atomic: once one machine wins, the source UID is no longer available to the
// others. A nil claim means that no messages are left in the inbox.
func (c *IMAPClient) ClaimNext(inbox, processingMailbox string) (*ClaimedMessage, error) {
	if _, err := c.client.Select(inbox, nil).Wait(); err != nil {
		return nil, fmt.Errorf("select inbox: %w", err)
	}
	search, err := c.client.UIDSearch(&imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagDeleted}}, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("search inbox: %w", err)
	}
	for _, uid := range search.AllUIDs() {
		moveData, err := c.client.Move(imap.UIDSetNum(uid), processingMailbox).Wait()
		if err != nil {
			// Another machine can legitimately win the race for this UID.
			continue
		}
		destUIDs, ok := moveData.DestUIDs.(imap.UIDSet)
		if !ok {
			return nil, errors.New("IMAP MOVE did not return destination UIDs")
		}
		uids, ok := destUIDs.Nums()
		if !ok || len(uids) != 1 {
			return nil, errors.New("IMAP MOVE returned an unexpected destination UID set")
		}
		return &ClaimedMessage{UID: uids[0]}, nil
	}
	return nil, nil
}

// FetchClaimedRaw retrieves BODY.PEEK[] without parsing or rewriting it, so
// the .eml file contains the original MIME octets supplied by the IMAP server.
func (c *IMAPClient) FetchClaimedRaw(processingMailbox string, uid imap.UID) ([]byte, error) {
	if _, err := c.client.Select(processingMailbox, nil).Wait(); err != nil {
		return nil, fmt.Errorf("select processing mailbox: %w", err)
	}
	section := &imap.FetchItemBodySection{Peek: true}
	messages, err := c.client.Fetch(imap.UIDSetNum(uid), &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{section},
	}).Collect()
	if err != nil {
		return nil, fmt.Errorf("fetch raw message: %w", err)
	}
	if len(messages) != 1 {
		return nil, fmt.Errorf("expected one claimed message, got %d", len(messages))
	}
	raw := wholeMessageBody(messages[0].BodySection)
	if raw == nil {
		return nil, errors.New("IMAP server returned no message body")
	}
	return append([]byte(nil), raw...), nil
}

func wholeMessageBody(sections map[*imap.FetchItemBodySection][]byte) []byte {
	for fetchedSection, data := range sections {
		if fetchedSection.Specifier == imap.PartSpecifierNone && len(fetchedSection.Part) == 0 && fetchedSection.Partial == nil && len(fetchedSection.HeaderFields) == 0 && len(fetchedSection.HeaderFieldsNot) == 0 {
			return data
		}
	}
	return nil
}

// DeleteClaimed deletes only the claimed UID, leaving unrelated processing
// messages untouched.
func (c *IMAPClient) DeleteClaimed(uid imap.UID) error {
	if err := c.client.Store(imap.UIDSetNum(uid), &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagDeleted},
	}, nil).Close(); err != nil {
		return fmt.Errorf("mark message deleted: %w", err)
	}
	if err := c.client.UIDExpunge(imap.UIDSetNum(uid)).Close(); err != nil {
		return fmt.Errorf("expunge message: %w", err)
	}
	return nil
}

// ReleaseClaim returns a message to INBOX if it couldn't be fully saved.
func (c *IMAPClient) ReleaseClaim(inbox string, uid imap.UID) error {
	// DeleteClaimed can fail after the server accepted STORE \Deleted but before
	// EXPUNGE. Clear that flag so the returned message remains eligible for a
	// later retry.
	if err := c.client.Store(imap.UIDSetNum(uid), &imap.StoreFlags{
		Op:     imap.StoreFlagsDel,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagDeleted},
	}, nil).Close(); err != nil {
		return fmt.Errorf("clear deleted flag: %w", err)
	}
	if _, err := c.client.Move(imap.UIDSetNum(uid), inbox).Wait(); err != nil {
		return fmt.Errorf("return claimed message to inbox: %w", err)
	}
	return nil
}

func (c *IMAPClient) Close() error {
	return c.client.Close()
}

// deadlineConn keeps the IMAP library's per-operation deadlines bounded by
// the service timeout, including long literal reads.
type deadlineConn struct {
	net.Conn
	timeout time.Duration
}

func (c *deadlineConn) Read(p []byte) (int, error) {
	if err := c.Conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, err
	}
	return c.Conn.Read(p)
}

func (c *deadlineConn) Write(p []byte) (int, error) {
	if err := c.Conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, err
	}
	return c.Conn.Write(p)
}
