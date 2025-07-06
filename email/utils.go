package email

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	telehtml "github.com/svanichkin/TelegramHTML"
)

func ExtractHeader(data []byte) (mail.Header, error) {

	r := bytes.NewReader(data)
	msg, err := message.Read(r)
	if err != nil {
		return mail.Header{}, fmt.Errorf("failed to read message: %w", err)
	}

	return mail.Header{Header: msg.Header}, nil

}

func HasAttachments(data []byte) bool {

	msg, err := message.Read(bytes.NewReader(data))
	if err != nil {
		return false
	}
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
		if err == io.EOF {
			break
		} else if err != nil {
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

func ExtractText(header mail.Header, data []byte) string {

	subj, _ := header.Subject()
	subj = "<b>" + cleanSubjectPrefix(subj) + "</b>\n\n"
	var html, plain strings.Builder
	msg, err := message.Read(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	if mr := msg.MultipartReader(); mr != nil {
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			} else if err != nil {
				break
			}
			ct := p.Header.Get("Content-Type")
			b, _ := io.ReadAll(p.Body)
			if strings.HasPrefix(ct, "text/html") {
				html.Write(b)
			} else if strings.HasPrefix(ct, "text/plain") {
				plain.Write(b)
			}
		}
	} else {
		b, _ := io.ReadAll(msg.Body)
		ct := msg.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "text/html") {
			html.Write(b)
		} else {
			plain.Write(b)
		}
	}
	if html.Len() > 0 {
		h := htmlToMarkdown(subj + telehtml.CleanTelegramHTML(html.String()))
		fmt.Print(h)
		return h
	}

	return plain.String()

}

func ExtractUnsubscribe(header mail.Header) []string {

	var result []string
	if val := header.Get("List-Unsubscribe"); val != "" {
		parts := strings.Split(val, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			p = strings.Trim(p, "<>")
			if strings.HasPrefix(p, "http") {
				result = append(result, p)
			}
		}
	}

	return result

}

func htmlToMarkdown(html string) string {

	converter := htmltomarkdown.NewConverter("", true, nil)
	md, err := converter.ConvertString(html)
	if err != nil {
		return html
	}

	return md
}

func cleanSubjectPrefix(name string) string {

	name = strings.TrimSpace(name)
	for {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "re:") {
			name = strings.TrimSpace(name[3:])
		} else if strings.HasPrefix(lower, "fw:") {
			name = strings.TrimSpace(name[3:])
		} else {
			break
		}
	}

	return name

}
