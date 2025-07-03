package file

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pkg/xattr"
)

func CleanFolderName(name string) string {

	var cleaned []rune
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '.' || r == '-' || r == '@' {
			cleaned = append(cleaned, r)
		} else {
			cleaned = append(cleaned, '_')
		}
	}

	return string(cleaned)

}

func CleanFileName(name string) string {

	// Удаление префиксов

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

	// Замена двоеточий и очистка

	name = strings.Replace(name, ":", "꞉", -1)
	name = strings.Replace(name, "/", "∕", -1)

	return name

}

func SetAttributes(path string, attrs map[string]string) error {

	for key, value := range attrs {
		if err := xattr.Set(path, key, []byte(value)); err != nil {
			return err
		}
	}

	return nil

}

func TrimFilenameToMaxBytes(s string, maxBytes int) string {

	if maxBytes <= 0 {
		return ""
	}

	var b []byte
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])

		if r == utf8.RuneError && size == 1 {
			if len(b)+3 > maxBytes { // U+FFFD занимает 3 байта
				break
			}
			b = append(b, '\xEF', '\xBF', '\xBD') // �
			i += size
		} else {
			if len(b)+size > maxBytes {
				break
			}
			b = append(b, s[i:i+size]...)
			i += size
		}
	}

	return string(b)

}
