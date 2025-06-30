package email

import (
	"bytes"
	"fmt"
	"mime"
	"strings"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

func ExtractHeader(data []byte) (mail.Header, error) {

	r := bytes.NewReader(data)
	msg, err := message.Read(r)
	if err != nil {
		return mail.Header{}, fmt.Errorf("failed to read message: %w", err)
	}

	return mail.Header{Header: msg.Header}, nil

}

func HasAttachments(msg *message.Entity) bool {

	mt, _, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mt, "multipart/") {
		return false
	}

	mr := msg.MultipartReader()
	if mr == nil {
		return false
	}

	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		if disp := p.Header.Get("Content-Disposition"); strings.HasPrefix(disp, "attachment") {
			return true
		}
	}

	return false

}

func ExtractAddresses(header mail.Header, field string) []string {

	addrs, _ := header.AddressList(field)
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		result = append(result, addr.Address)
	}

	return result

}

func ExtractDate(header mail.Header) time.Time {

	if t, err := header.Date(); err == nil {
		return t
	}

	return time.Now()

}
