package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const randomStringCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

var (
	configPath    = "config.conf"
	ipAddress     string
	port          string
	rootDirectory string
	indexFiles    []string
	notFoundPage  string
	forbiddenPage string
)

func loadConfig() {
	// Read the configuration file
	content, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Error reading configuration file: %v", err)
	}

	// Parse the configuration content
	lines := strings.Split(string(content), "\n")
	for lineNumber, line := range lines {
		line = strings.TrimSpace(line) // Trim leading and trailing whitespace

		// Skip empty lines and comment lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			log.Fatalf("Invalid configuration line (line %d): %s", lineNumber+1, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "ip-address":
			ipAddress = value
		case "port":
			port = value
		case "root":
			rootDirectory = value
		case "index":
			indexFiles = strings.Split(value, ";")
		case "404-error":
			notFoundPage = value
		case "403-error":
			forbiddenPage = value
		default:
			log.Printf("Warning: Unknown configuration item (line %d): %s", lineNumber+1, line)
		}
	}
}

func generateRandomString(length int) string {
	// Generate a random string of the specified length
	rand.Seed(time.Now().UnixNano())
	randomBytes := make([]byte, length)
	for i := range randomBytes {
		randomBytes[i] = randomStringCharset[rand.Intn(len(randomStringCharset))]
	}
	return string(randomBytes)
}

func logRequestDetails(r *http.Request, filePath string, bytesSent int64) {
	// Log request details, including a random string
	clientIP := strings.Split(r.RemoteAddr, ":")[0]
	requestTime := time.Now().Format("2006-01-02 15:04:05")
	randomString := generateRandomString(16)
	logDetails := fmt.Sprintf("%s | ClientIP: %s | Port: %s | File: %s | Time: %s | BytesSent: %d\n", randomString, clientIP, port, filePath, requestTime, bytesSent)

	// Print to console
	fmt.Print(logDetails)

	// Write to log file
	logFile, err := os.OpenFile("latest.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Error opening log file: %v", err)
		return
	}
	defer logFile.Close()

	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.Print(logDetails)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// Handle the request
	filePath := r.URL.Path
	if filePath == "/" {
		// If the root directory is requested, try using the default index files
		for _, indexFile := range indexFiles {
			if _, err := os.Stat(rootDirectory + indexFile); err == nil {
				filePath = indexFile
				break
			}
		}
	}

	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}

	fullPath := rootDirectory + filePath

	// Check if the file or directory exists
	fileInfo, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		// If the file doesn't exist, serve the 404 page
		http.ServeFile(w, r, rootDirectory+notFoundPage)
		return
	}

	// If a directory is requested, use http.ServeFile to handle it
	if fileInfo.IsDir() {
		http.ServeFile(w, r, fullPath)
		return
	}

	// If a file is requested, use http.ServeContent to support multi-threaded downloads
	file, err := os.Open(fullPath)
	if err != nil {
		// If the file cannot be opened, serve the 403 page
		http.ServeFile(w, r, rootDirectory+forbiddenPage)
		return
	}
	defer file.Close()

	// Set the Content-Type based on the file extension
	switch {
	case strings.HasSuffix(filePath, ".html"):
		w.Header().Set("Content-Type", "text/html")
	case strings.HasSuffix(filePath, ".css"):
		w.Header().Set("Content-Type", "text/css")
	case strings.HasSuffix(filePath, ".js"):
		w.Header().Set("Content-Type", "application/javascript")
	}

	// Use http.ServeContent to transmit the file content, supporting multi-threaded downloads
	http.ServeContent(w, r, fileInfo.Name(), fileInfo.ModTime(), file)

	// Update bytesSent after successful transmission
	bytesSent := fileInfo.Size()
	logRequestDetails(r, filePath, bytesSent)
}

func main() {
    loadConfig() // Load configuration

    // Set up the handler function
    http.HandleFunc("/", handleRequest)

    // Handle IPv6 addresses
    addr := ipAddress
    if strings.Contains(ipAddress, ":") {
        addr = fmt.Sprintf("[%s]", ipAddress) // Enclose IPv6 address in square brackets
    }
    serverAddr := fmt.Sprintf("%s:%s", addr, port)

    srv := &http.Server{
        Addr:           serverAddr,
        Handler:        nil, // Use http.DefaultServeMux
        ReadTimeout:    10 * time.Second,
        WriteTimeout:   10 * time.Second,
        IdleTimeout:    15 * time.Second,
        MaxHeaderBytes: 1 << 20,
    }

    fmt.Printf("Server running at http://%s\n", serverAddr)
    pid := os.Getpid()
    fmt.Printf("PID: %d\n", pid)

    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("Error starting server: %v", err)
        }
    }()

    // Capture Ctrl+C signal
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)

    // Wait for Ctrl+C signal
    <-c

    // Print green exit message
    fmt.Print("\033[1;32mServer exiting...\033[0m\n")

    // Gracefully shut down the server
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := srv.Shutdown(ctx); err != nil {
        log.Fatalf("Error shutting down server: %v", err)
    }
}
