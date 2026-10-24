package conf

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Addresses             string `json:"addresses"`
	Passwords             string `json:"passwords"`
	Keys                  string `json:"keys"`
	Folder                string `json:"folder"`
	RemoteAIToken         string `json:"remoteai"`
	IMAPServer            string `json:"imapServer"`
	IMAPProcessingMailbox string `json:"imapProcessingMailbox"`
}

// RemoteAITokenValue returns the configured token. When Keys is set, RemoteAIToken
// is treated as a filename in that directory, keeping the token out of config.json.
func (c Config) RemoteAITokenValue() (string, error) {
	if c.RemoteAIToken == "" || c.Keys == "" {
		return c.RemoteAIToken, nil
	}

	keyFile := filepath.Join(c.Keys, c.RemoteAIToken)
	file, err := os.Open(keyFile)
	if err != nil {
		return "", fmt.Errorf("read RemoteAI key file %q: %w", keyFile, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read RemoteAI key file %q: %w", keyFile, err)
		}
		return "", fmt.Errorf("RemoteAI key file %q is empty", keyFile)
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read RemoteAI key file %q: %w", keyFile, err)
	}

	return strings.TrimSpace(scanner.Text()), nil
}

func GetConfigFilePath() (string, error) {

	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exePath)

	return filepath.Join(exeDir, "config.json"), nil

}

func Init() (Config, error) {

	configFile, err := GetConfigFilePath()
	if err != nil {
		return Config{}, err
	}
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		var servers string
		var passwords string
		fmt.Print("Configuration file not found. Please enter the path to the folder: ")
		_, err := fmt.Scanln(&servers)
		if err != nil {
			fmt.Println("Error reading input:", err)
			return Config{}, err
		}

		if err := CreateConfig(configFile, servers, passwords); err != nil {
			fmt.Println("Error creating configuration file:", err)
			return Config{}, err
		}
		fmt.Println("Created configuration file:", configFile)
	}
	config, err := ReadConfig(configFile)
	if err != nil {
		fmt.Println("Error reading configuration file:", err)
		return Config{}, err
	}

	return config, nil

}

func ReadConfig(configFile string) (Config, error) {

	file, err := os.Open(configFile)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	var config Config
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return Config{}, err
	}

	return config, nil

}

func CreateConfig(configFile, servers, passwords string) error {

	config := Config{Addresses: servers, Passwords: passwords}
	file, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)

	return encoder.Encode(config)

}
