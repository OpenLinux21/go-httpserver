package config

import (
	"fmt"
	"os"
	"strings"
)

const defaultConfig = `# Basic Configuration
ip-address = 0.0.0.0
port = 8081
root = ./website
index = index.html;index.htm
404-error = 404.html
403-error = 403.html

# HTTPS Configuration
enable-https = true
cert-file = server.crt
key-file = server.key
https-port = 8443`

type Config struct {
	IPAddress     string
	Port          string
	RootDirectory string
	IndexFiles    []string
	NotFoundPage  string
	ForbiddenPage string
	EnableHTTPS   bool
	CertFile      string
	KeyFile       string
	HTTPSPort     string
}

var GlobalConfig Config

func CreateDefaultConfig() error {
	configPath := "config.conf"
	if _, err := os.Stat(configPath); err == nil {
		return nil
	}

	if err := os.MkdirAll("website", 0755); err != nil {
		return fmt.Errorf("failed to create website directory: %v", err)
	}

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to create default config file: %v", err)
	}

	fmt.Printf("Created default config file: %s\n", configPath)
	return nil
}

func LoadConfig() error {
	if err := CreateDefaultConfig(); err != nil {
		return fmt.Errorf("error creating default config: %v", err)
	}

	content, err := os.ReadFile("config.conf")
	if err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	for lineNumber, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("config line format error (line %d): %s", lineNumber+1, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "ip-address":
			GlobalConfig.IPAddress = value
		case "port":
			GlobalConfig.Port = value
		case "root":
			GlobalConfig.RootDirectory = value
		case "index":
			GlobalConfig.IndexFiles = strings.Split(value, ";")
		case "404-error":
			GlobalConfig.NotFoundPage = value
		case "403-error":
			GlobalConfig.ForbiddenPage = value
		case "enable-https":
			GlobalConfig.EnableHTTPS = strings.ToLower(value) == "true"
		case "cert-file":
			GlobalConfig.CertFile = value
		case "key-file":
			GlobalConfig.KeyFile = value
		case "https-port":
			GlobalConfig.HTTPSPort = value
		default:
			fmt.Printf("Warning: Unknown config item (line %d): %s\n", lineNumber+1, line)
		}
	}

	return nil
}
