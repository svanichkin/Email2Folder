package email

import (
	"bytes"
	"errors"
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

	val := header.Get(field)
	if val == "" {
		return nil
	}
	var result []string
	addrs, err := mail.ParseAddressList(val)
	if err == nil && len(addrs) > 0 {
		result = make([]string, 0, len(addrs))
		for _, addr := range addrs {
			result = append(result, addr.Address)
		}
	} else {
		result = extractEmailsFromString(val)
	}
	result = removeDoubles(result)

	return result
}

func removeDoubles(input []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(input))
	for _, s := range input {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}
	return result
}

func extractEmailsFromString(s string) []string {
	parts := strings.Fields(s)
	var res []string
	for _, part := range parts {
		if strings.Contains(part, "@") {
			res = append(res, strings.Trim(part, "<>"))
		}
	}
	return res
}

func ExtractDate(header mail.Header) time.Time {

	if t, err := header.Date(); err == nil {
		return t
	}

	return time.Now()

}

func ExtractText(header mail.Header, data []byte) (string, error) {

	subj, _ := header.Subject()
	subj = "<b>" + cleanSubjectPrefix(subj) + "</b>\n\n"
	var html, plain strings.Builder

	msg, err := message.Read(bytes.NewReader(data))
	if err != nil {
		return "", err
	}

	var processPart func(*message.Entity)
	processPart = func(entity *message.Entity) {
		if mr := entity.MultipartReader(); mr != nil {
			for {
				p, err := mr.NextPart()
				if err == io.EOF {
					break
				} else if err != nil {
					if !strings.Contains(err.Error(), "unknown charset") {
						break
					}
				}
				processPart(p)
			}
		} else {
			b, _ := io.ReadAll(entity.Body)
			ct := entity.Header.Get("Content-Type")
			mt, _, _ := mime.ParseMediaType(ct)
			switch mt {
			case "text/html":
				html.Write(b)
			case "text/plain":
				plain.Write(b)
			}
		}
	}

	processPart(msg)

	if html.Len() > 0 {
		h := htmlToMarkdown(subj + telehtml.CleanTelegramHTML(html.String()))
		return h, nil
	}

	if plain.Len() > 0 {
		return plain.String(), nil
	}

	return "", errors.New("no text found in message")
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

// not working
func NormalizeBodyLineEndings(data []byte) []byte {

	sep := []byte("\r\n\r\n")
	idx := bytes.Index(data, sep)
	if idx < 0 {
		return data
	}
	headerPart := data[:idx+len(sep)]
	bodyPart := data[idx+len(sep):]
	bodyFixed := bytes.ReplaceAll(bodyPart, []byte("\r\n"), []byte("\n"))

	return append(headerPart, bodyFixed...)

}
