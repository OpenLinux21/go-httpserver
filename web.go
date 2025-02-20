package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
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
	logDetails := fmt.Sprintf("%s | ClientIP: %s | Port: %s | File: %s | Time: %s | BytesSent: %d\n",
		randomString, clientIP, port, filePath, requestTime, bytesSent)

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

func autoIndex(w http.ResponseWriter, r *http.Request, directoryPath string) {
	entries, err := os.ReadDir(directoryPath)
	if err != nil {
		http.Error(w, "Unable to list directory", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<html><head><title>Index of %s</title>", r.URL.Path)
	fmt.Fprintf(w, `<style>
		body { font-family: monospace; }
		table { width: 100%%; border-collapse: collapse; }
		th, td { text-align: left; padding: 5px; border-bottom: 1px solid #ddd; }
		a { text-decoration: none; }
		</style>`)
	fmt.Fprintf(w, "</head><body>")
	fmt.Fprintf(w, "<h1>Index of %s</h1>", r.URL.Path)
	fmt.Fprintf(w, "<table>")
	fmt.Fprintf(w, "<tr><th>Name</th><th>Last Modified</th><th>Size</th></tr>")

	// If not in the root directory, add a link to the parent directory
	if r.URL.Path != "/" {
		parent := ".."
		fmt.Fprintf(w, `<tr>
			<td><a href="%s">%s</a></td>
			<td></td>
			<td>-</td>
			</tr>`, parent, "Parent Directory")
	}

	for _, entry := range entries {
		name := entry.Name()
		// URL-encode the file or directory name to handle special characters like '#'
		encodedName := url.PathEscape(name)

		// Construct the link using the encoded name
		link := r.URL.Path
		if !strings.HasSuffix(link, "/") {
			link += "/"
		}
		link += encodedName

		// Append a slash to the display name and link if it's a directory
		displayName := name
		if entry.IsDir() {
			displayName += "/"
			link += "/"
		}

		// Get file info to display last modified time and size
		info, err := entry.Info()
		modTime := ""
		size := ""
		if err == nil {
			modTime = info.ModTime().Format("2006-01-02 15:04")
			if info.IsDir() {
				size = "-"
			} else {
				size = fmt.Sprintf("%d", info.Size())
			}
		}
		fmt.Fprintf(w, `<tr>
			<td><a href="%s">%s</a></td>
			<td>%s</td>
			<td>%s</td>
			</tr>`, link, displayName, modTime, size)
	}
	fmt.Fprintf(w, "</table>")
	fmt.Fprintf(w, "<hr><address>Go HTTP Server</address>")
	fmt.Fprintf(w, "</body></html>")
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// Decode the URL path to handle special characters (like '#' encoded as %23)
	filePath, err := url.PathUnescape(r.URL.Path)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	fullPath := rootDirectory + filePath

	// If the request is for the root directory, try to find the index file defined in the configuration
	if filePath == "/" {
		for _, indexFile := range indexFiles {
			indexFullPath := filepath.Join(rootDirectory, indexFile)
			if _, err := os.Stat(indexFullPath); err == nil {
				filePath = indexFile
				fullPath = filepath.Join(rootDirectory, filePath)
				break
			}
		}
	}

	// Ensure the path starts with "/"
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
		fullPath = rootDirectory + filePath
	}

	// Check if the file or directory exists
	fileInfo, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		// Return the 404 page if the file does not exist
		http.ServeFile(w, r, filepath.Join(rootDirectory, notFoundPage))
		return
	}

	// If the request is for a directory
	if fileInfo.IsDir() {
		// Try to find an index file in the directory
		foundIndex := false
		for _, indexFile := range indexFiles {
			indexFullPath := filepath.Join(fullPath, indexFile)
			if info, err := os.Stat(indexFullPath); err == nil && !info.IsDir() {
				http.ServeFile(w, r, indexFullPath)
				foundIndex = true
				break
			}
		}
		// If no index file is found, generate the autoindex page
		if !foundIndex {
			autoIndex(w, r, fullPath)
			return
		}
	}

	// If the request is for a file, try to open and serve its content
	file, err := os.Open(fullPath)
	if err != nil {
		// Return the 403 page if the file cannot be opened
		http.ServeFile(w, r, filepath.Join(rootDirectory, forbiddenPage))
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

	// Use http.ServeContent to support multi-threaded downloads
	http.ServeContent(w, r, fileInfo.Name(), fileInfo.ModTime(), file)

	// Log the request
	bytesSent := fileInfo.Size()
	logRequestDetails(r, filePath, bytesSent)
}

func main() {
	loadConfig() // Load configuration

	// Set the request handler
	http.HandleFunc("/", handleRequest)

	// Handle IPv6 addresses
	addr := ipAddress
	if strings.Contains(ipAddress, ":") {
		addr = fmt.Sprintf("[%s]", ipAddress) // IPv6 addresses need to be enclosed in square brackets
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

	// Wait for signal
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
