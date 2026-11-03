package config

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultConfig = `# Basic Configuration
ip-address = 0.0.0.0
port = 8081
root = ./website
index = index.html;index.htm
404-error = 404.html
403-error = 403.html
trusted-proxies =

# HTTPS Configuration
enable-https = false
cert-file = server.crt
key-file = server.key
https-port = 8443

# Logging Configuration
log-buffer-size = 1MiB
log-flush-threshold = 75
log-flush-interval = 5
log-max-entries = 12500`

type Config struct {
	IPAddress      string
	Port           string
	RootDirectory  string
	IndexFiles     []string
	NotFoundPage   string
	ForbiddenPage  string
	TrustedProxies []string
	EnableHTTPS    bool
	CertFile       string
	KeyFile        string
	HTTPSPort      string

	LogBufferSize     int // in-memory log buffer capacity in bytes
	LogFlushThreshold int // buffer fill percentage that triggers a flush
	LogFlushInterval  int // seconds between periodic flushes; 0 disables
	LogMaxEntries     int // entries kept in the log file before compaction
}

func defaults() Config {
	return Config{
		IPAddress:     "0.0.0.0",
		Port:          "8081",
		RootDirectory: "./website",
		IndexFiles:    []string{"index.html", "index.htm"},
		NotFoundPage:  "404.html",
		ForbiddenPage: "403.html",
		HTTPSPort:     "8443",

		LogBufferSize:     1 << 20,
		LogFlushThreshold: 75,
		LogFlushInterval:  5,
		LogMaxEntries:     12500,
	}
}

func CreateDefaultConfig() error {
	if _, err := os.Stat("config.conf"); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check config file: %w", err)
	}

	if err := os.MkdirAll("website", 0o755); err != nil {
		return fmt.Errorf("create website directory: %w", err)
	}
	if err := os.WriteFile("config.conf", []byte(defaultConfig), 0o644); err != nil {
		return fmt.Errorf("create default config file: %w", err)
	}

	fmt.Println("Created default config file: config.conf")
	return nil
}

func LoadConfig() (Config, error) {
	if err := CreateDefaultConfig(); err != nil {
		return Config{}, err
	}

	file, err := os.Open("config.conf")
	if err != nil {
		return Config{}, fmt.Errorf("open config file: %w", err)
	}
	defer file.Close()

	cfg, err := parse(file)
	if err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

func parse(r io.Reader) (Config, error) {
	cfg := defaults()
	scanner := bufio.NewScanner(r)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("config line %d has no '=': %q", lineNumber, line)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)

		switch key {
		case "ip-address":
			cfg.IPAddress = value
		case "port":
			cfg.Port = value
		case "root":
			cfg.RootDirectory = value
		case "index":
			cfg.IndexFiles = splitList(value)
		case "404-error":
			cfg.NotFoundPage = value
		case "403-error":
			cfg.ForbiddenPage = value
		case "trusted-proxies":
			cfg.TrustedProxies = splitList(value)
		case "enable-https":
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				return Config{}, fmt.Errorf("config line %d: invalid enable-https value %q", lineNumber, value)
			}
			cfg.EnableHTTPS = enabled
		case "cert-file":
			cfg.CertFile = value
		case "key-file":
			cfg.KeyFile = value
		case "https-port":
			cfg.HTTPSPort = value
		case "log-buffer-size":
			size, err := parseByteSize(value)
			if err != nil {
				return Config{}, fmt.Errorf("config line %d: invalid log-buffer-size %q: %w", lineNumber, value, err)
			}
			cfg.LogBufferSize = int(size)
		case "log-flush-threshold":
			percent, err := strconv.Atoi(value)
			if err != nil || percent < 0 {
				return Config{}, fmt.Errorf("config line %d: invalid log-flush-threshold %q", lineNumber, value)
			}
			cfg.LogFlushThreshold = percent
		case "log-flush-interval":
			seconds, err := strconv.Atoi(value)
			if err != nil || seconds < 0 {
				return Config{}, fmt.Errorf("config line %d: invalid log-flush-interval %q", lineNumber, value)
			}
			cfg.LogFlushInterval = seconds
		case "log-max-entries":
			count, err := strconv.Atoi(value)
			if err != nil || count < 0 {
				return Config{}, fmt.Errorf("config line %d: invalid log-max-entries %q", lineNumber, value)
			}
			cfg.LogMaxEntries = count
		default:
			return Config{}, fmt.Errorf("config line %d: unknown option %q", lineNumber, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	return cfg, nil
}

func splitList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ";")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

var byteSizeSuffixes = map[string]int64{
	"":    1,
	"b":   1,
	"kib": 1 << 10,
	"mib": 1 << 20,
	"gib": 1 << 30,
}

// parseByteSize accepts plain byte counts ("1048576") or suffixed values
// ("256KiB", "1 MiB"). Suffixes are case-insensitive.
func parseByteSize(value string) (int64, error) {
	text := strings.ToLower(strings.TrimSpace(value))
	digitsEnd := 0
	for digitsEnd < len(text) && text[digitsEnd] >= '0' && text[digitsEnd] <= '9' {
		digitsEnd++
	}
	digits, suffix := text[:digitsEnd], strings.TrimSpace(text[digitsEnd:])
	if digits == "" {
		return 0, fmt.Errorf("expected a positive size")
	}
	multiplier, ok := byteSizeSuffixes[suffix]
	if !ok {
		return 0, fmt.Errorf("unknown suffix %q (use B, KiB, MiB, or GiB)", suffix)
	}
	size, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || size <= 0 {
		return 0, fmt.Errorf("expected a positive size")
	}
	if size > math.MaxInt/multiplier {
		return 0, fmt.Errorf("size exceeds supported range")
	}
	return size * multiplier, nil
}

// Logging configuration bounds. The in-memory buffer and entry cap keep both
// memory usage and the log file size bounded regardless of traffic.
const (
	minLogBufferSize     = 256 << 10 // 256KiB
	maxLogBufferSize     = 64 << 20  // 64MiB
	minLogFlushThreshold = 50
	maxLogFlushThreshold = 95
	maxLogFlushInterval  = 3600 // seconds; 0 disables the periodic flush
	minLogMaxEntries     = 250
	maxLogMaxEntries     = 10_000_000
)

func (cfg Config) Validate() error {
	if cfg.IPAddress == "" {
		return fmt.Errorf("ip-address must not be empty")
	}
	httpPort, err := parsePort("port", cfg.Port)
	if err != nil {
		return err
	}
	if cfg.EnableHTTPS {
		httpsPort, err := parsePort("https-port", cfg.HTTPSPort)
		if err != nil {
			return err
		}
		if httpPort == httpsPort {
			return fmt.Errorf("port and https-port must differ")
		}
		if _, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile); err != nil {
			return fmt.Errorf("load TLS certificate and key: %w", err)
		}
	}

	info, err := os.Stat(cfg.RootDirectory)
	if err != nil {
		return fmt.Errorf("root directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("root %q is not a directory", cfg.RootDirectory)
	}

	for _, name := range append(append([]string{}, cfg.IndexFiles...), cfg.NotFoundPage, cfg.ForbiddenPage) {
		if name != "" && (!fs.ValidPath(filepath.ToSlash(name)) || name == ".") {
			return fmt.Errorf("configured file %q must be a relative path within root", name)
		}
	}
	for _, proxy := range cfg.TrustedProxies {
		if net.ParseIP(proxy) == nil {
			if _, _, err := net.ParseCIDR(proxy); err != nil {
				return fmt.Errorf("invalid trusted proxy %q", proxy)
			}
		}
	}

	switch {
	case cfg.LogBufferSize < minLogBufferSize || cfg.LogBufferSize > maxLogBufferSize:
		return fmt.Errorf("log-buffer-size must be between 256KiB and 64MiB")
	case cfg.LogFlushThreshold < minLogFlushThreshold || cfg.LogFlushThreshold > maxLogFlushThreshold:
		return fmt.Errorf("log-flush-threshold must be between %d and %d percent", minLogFlushThreshold, maxLogFlushThreshold)
	case cfg.LogFlushInterval < 0 || cfg.LogFlushInterval > maxLogFlushInterval:
		return fmt.Errorf("log-flush-interval must be between 0 (disabled) and %d seconds", maxLogFlushInterval)
	case cfg.LogMaxEntries < minLogMaxEntries || cfg.LogMaxEntries > maxLogMaxEntries:
		return fmt.Errorf("log-max-entries must be between %d and %d", minLogMaxEntries, maxLogMaxEntries)
	}
	return nil
}

func parsePort(label, value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s must be between 1 and 65535", label)
	}
	return port, nil
}
