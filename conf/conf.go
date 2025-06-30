package conf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	EmailAdresses string `json:"emailAdresses"`
	Passwords     string `json:"passwords"`
	Folder        string `json:"folder"`
}

func GetConfigFilePath() (string, error) {

	configDir := "/etc/email2folder"
	if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
		return "", err
	}

	return filepath.Join(configDir, "config.json"), nil

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

	config := Config{EmailAdresses: servers, Passwords: passwords}

	file, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(config)

}
