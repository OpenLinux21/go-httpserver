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

	"github.com/gin-gonic/gin"
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
	// Add HTTPS related configuration items
	enableHTTPS bool
	certFile    string
	keyFile     string
	httpsPort   string // Add HTTPS port configuration
)

func loadConfig() {
	// Read config file
	content, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	// Parse config content
	lines := strings.Split(string(content), "\n")
	for lineNumber, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			log.Fatalf("Config line format error (line %d): %s", lineNumber+1, line)
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
			// Add HTTPS configuration item handling
		case "enable-https":
			enableHTTPS = strings.ToLower(value) == "true"
		case "cert-file":
			certFile = value
		case "key-file":
			keyFile = value
		case "https-port": // Add HTTPS port configuration handling
			httpsPort = value
		default:
			log.Printf("Warning: Unknown config item (line %d): %s", lineNumber+1, line)
		}
	}
}

func generateRandomString(length int) string {
	// Generate a random string of specified length
	rand.Seed(time.Now().UnixNano())
	randomBytes := make([]byte, length)
	for i := range randomBytes {
		randomBytes[i] = randomStringCharset[rand.Intn(len(randomStringCharset))]
	}
	return string(randomBytes)
}

func logRequestDetails(r *http.Request, filePath string, bytesSent int64) {
	// Log request details to latest.log file
	clientIP := strings.Split(r.RemoteAddr, ":")[0]
	requestTime := time.Now().Format("2006-01-02 15:04:05")
	randomString := generateRandomString(16)
	logDetails := fmt.Sprintf("%s | ClientIP: %s | Port: %s | File: %s | Time: %s | BytesSent: %d\n",
		randomString, clientIP, port, filePath, requestTime, bytesSent)

	logFile, err := os.OpenFile("latest.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Error opening log file: %v", err)
		return
	}
	defer logFile.Close()

	if _, err := io.WriteString(logFile, logDetails); err != nil {
		log.Printf("Error writing to log file: %v", err)
	}
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
		encodedName := url.PathEscape(name)

		link := r.URL.Path
		if !strings.HasSuffix(link, "/") {
			link += "/"
		}
		link += encodedName

		displayName := name
		if entry.IsDir() {
			displayName += "/"
			link += "/"
		}

		info, err := entry.Info()
		modTime, size := "", ""
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

// Add basic security headers
func addSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	// Modify CSP policy to allow inline styles
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'")
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// Add security headers
	addSecurityHeaders(w)

	// Declare support for range requests
	w.Header().Set("Accept-Ranges", "bytes")

	// Decode URL path, handle special characters
	filePath, err := url.PathUnescape(r.URL.Path)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	fullPath := rootDirectory + filePath

	// Root directory index file logic
	if filePath == "/" {
		for _, idx := range indexFiles {
			if _, err := os.Stat(filepath.Join(rootDirectory, idx)); err == nil {
				filePath, fullPath = idx, filepath.Join(rootDirectory, idx)
				break
			}
		}
	}

	// Ensure path starts with "/"
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
		fullPath = rootDirectory + filePath
	}

	// Check if file or directory exists
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		http.ServeFile(w, r, filepath.Join(rootDirectory, notFoundPage))
		return
	}

	// Directory handling: prefer index file, otherwise auto-generate directory listing
	if info.IsDir() {
		for _, idx := range indexFiles {
			if fi, err := os.Stat(filepath.Join(fullPath, idx)); err == nil && !fi.IsDir() {
				http.ServeFile(w, r, filepath.Join(fullPath, idx))
				return
			}
		}
		autoIndex(w, r, fullPath)
		return
	}

	// File handling
	file, err := os.Open(fullPath)
	if err != nil {
		http.ServeFile(w, r, filepath.Join(rootDirectory, forbiddenPage))
		return
	}
	defer file.Close()

	// Set MIME type based on extension
	switch {
	case strings.HasSuffix(filePath, ".html"):
		w.Header().Set("Content-Type", "text/html")
	case strings.HasSuffix(filePath, ".css"):
		w.Header().Set("Content-Type", "text/css")
	case strings.HasSuffix(filePath, ".js"):
		w.Header().Set("Content-Type", "application/javascript")
	}

	// Use ServeContent to support Range requests, allowing clients to download different byte ranges in parallel
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)

	// Log request
	logRequestDetails(r, filePath, info.Size())
}

func main() {
	loadConfig()

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.Any("/*filepath", func(c *gin.Context) {
		handleRequest(c.Writer, c.Request)
	})

	addr := ipAddress
	if strings.Contains(ipAddress, ":") {
		addr = fmt.Sprintf("[%s]", ipAddress)
	}
	serverAddr := fmt.Sprintf("%s:%s", addr, port)

	srv := &http.Server{
		Addr:           serverAddr,
		Handler:        router,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    15 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	fmt.Printf("Server running at http://%s\n", serverAddr)
	fmt.Printf("PID: %d\n", os.Getpid())

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server start error: %v", err)
		}
	}()

	// Modify HTTPS support logic
	if enableHTTPS {
		if certFile == "" || keyFile == "" {
			log.Fatal("HTTPS is enabled but cert-file or key-file is not specified in config")
		}

		// Force using https-port specified in the configuration file
		if httpsPort == "" {
			log.Fatal("HTTPS is enabled but https-port is not specified in config")
		}

		go func() {
			httpsAddr := fmt.Sprintf("%s:%s", addr, httpsPort)
			httpsSrv := &http.Server{
				Addr:           httpsAddr,
				Handler:        router,
				ReadTimeout:    10 * time.Second,
				WriteTimeout:   10 * time.Second,
				IdleTimeout:    15 * time.Second,
				MaxHeaderBytes: 1 << 20,
			}

			fmt.Printf("HTTPS server running at https://%s\n", httpsAddr)
			if err := httpsSrv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				log.Printf("HTTPS server error: %v", err)
			}
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	fmt.Print("\033[1;32mServer exiting...\033[0m\n")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}
}
