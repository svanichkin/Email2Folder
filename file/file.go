package file

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/xattr"
)

func FindEmailAddresses(rootPath string) (map[string]string, error) {

	files := make(map[string]string)

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			sshFile := filepath.Join(path, "email")
			if _, err := os.Stat(sshFile); err == nil {
				file, err := os.Open(sshFile)
				if err != nil {
					return err
				}
				defer file.Close()

				scanner := bufio.NewScanner(file)
				if scanner.Scan() {
					ip := scanner.Text()
					folderName := filepath.Base(path)
					files[folderName] = ip
				}
			}
		}
		return nil
	})

	return files, err

}

func FindPasswordFiles(rootPath string, devices []string) (map[string]map[string]string, error) {

	passwordFiles := make(map[string]map[string]string)
	contains := func(device string, devices []string) bool {
		for _, d := range devices {
			if d == device {
				return true
			}
		}
		return false
	}
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			folderName := filepath.Base(filepath.Dir(path))

			if !contains(folderName, devices) {
				return nil
			}

			fileName := filepath.Base(path)

			if strings.HasPrefix(fileName, ".") {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			if scanner.Scan() {
				password := scanner.Text()
				if _, exists := passwordFiles[folderName]; !exists {
					passwordFiles[folderName] = make(map[string]string)
				}
				passwordFiles[folderName][fileName] = password
			}
		}
		return nil
	})

	return passwordFiles, err

}

func CreateNewFolder(folderPath string, attrs map[string]string) (string, error) {

	if err := os.MkdirAll(folderPath, 0755); err != nil {
		return "", err
	}
	if err := SetAttributes(folderPath, attrs); err != nil {
		os.RemoveAll(folderPath)
		return "", fmt.Errorf("xattr set error: %w", err)
	}

	return folderPath, nil

}

func FindFoldeArttrFrom(folderPath, fromKey string) (string, error) {

	var foundPath string
	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() || path == folderPath {
			return nil
		}
		attr, err := xattr.Get(path, "from")
		if err != nil {
			return nil
		}
		for _, val := range strings.Split(string(attr), ",") {
			val = strings.TrimSpace(val)
			if val == fromKey || strings.HasSuffix(fromKey, val) {
				foundPath = path
				return filepath.SkipDir
			}
		}
		return nil
	})
	if foundPath != "" {
		return foundPath, nil
	}

	return "", fmt.Errorf("folder not found: %w", err)

}
