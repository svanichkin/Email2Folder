package file

import (
	"strings"
	"unicode"

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

	var cleaned []rune
	allowed := "!\"#$%&'()*+,-./;<=>?@[\\]^_`{|}~ "
	for _, r := range name {
		if r == ':' {
			cleaned = append(cleaned, '꞉') // U+A789
		} else if r == '/' {
			cleaned = append(cleaned, '-')
		} else if unicode.IsLetter(r) || unicode.IsNumber(r) || strings.ContainsRune(allowed, r) {
			cleaned = append(cleaned, r)
		} else {
			cleaned = append(cleaned, '_')
		}
	}

	// Обрезка длины

	result := string(cleaned)
	if len(result) > 100 {
		result = result[:100]
	}

	return result

}

func SetAttributes(path string, attrs map[string]string) error {

	for key, value := range attrs {
		if err := xattr.Set(path, key, []byte(value)); err != nil {
			return err
		}
	}

	return nil

}
