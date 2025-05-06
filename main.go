package main

import (
	"compress/gzip"
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

// Default configuration content
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

// Create default config file if not exists
func createDefaultConfig() error {
	// Check if config file already exists
	if _, err := os.Stat(configPath); err == nil {
		return nil // File exists, do nothing
	}

	// Create website directory if not exists
	if err := os.MkdirAll("website", 0755); err != nil {
		return fmt.Errorf("failed to create website directory: %v", err)
	}

	// Write default config to file
	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to create default config file: %v", err)
	}

	fmt.Printf("Created default config file: %s\n", configPath)
	return nil
}

func loadConfig() {
	// Try to create default config if not exists
	if err := createDefaultConfig(); err != nil {
		log.Fatalf("Error creating default config: %v", err)
	}

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
	fmt.Fprintf(w, "<!DOCTYPE HTML PUBLIC \"-//W3C//DTD HTML 3.2 Final//EN\">\n")
	fmt.Fprintf(w, "<html>\n<head>\n<title>Index of %s</title>\n", r.URL.Path)
	fmt.Fprintf(w, `<style>
        body { font-family: Arial, sans-serif; }
        h1 { font-size: 1.5em; margin: 0.5em 0; }
        table { width: 100%%; border-collapse: collapse; font-family: monospace; }
        th { text-align: left; padding: 0.5em 1em; background: #e4e4e4; border-bottom: 1px solid #ccc; }
        td { padding: 0.25em 1em; }
        tr:hover td { background: #f4f4f4; }
        a { text-decoration: none; color: #00e; }
        a:hover { text-decoration: underline; color: #00f; }
        hr { border: 0; border-top: 1px solid #ccc; margin: 1em 0; }
        .name-cell { min-width: 35%%; }
        .date-cell { min-width: 20%%; }
        .size-cell { min-width: 10%%; }
        .icon { width: 20px; height: 20px; vertical-align: middle; margin-right: 5px; }
        .parent-dir { color: #666; }
        .dir-name { font-weight: bold; }
        address { font-size: 0.8em; font-style: italic; color: #666; margin-top: 1em; }
    </style>`)
	fmt.Fprintf(w, "</head>\n<body>\n")
	fmt.Fprintf(w, "<h1>Index of %s</h1>\n", r.URL.Path)
	fmt.Fprintf(w, "<table>\n")
	fmt.Fprintf(w, "<tr><th class=\"name-cell\">Name</th><th class=\"date-cell\">Last modified</th><th class=\"size-cell\">Size</th></tr>\n")
	fmt.Fprintf(w, "<tr><th colspan=\"3\"><hr></th></tr>\n")

	if r.URL.Path != "/" {
		parent := ".."
		fmt.Fprintf(w, `<tr>
            <td class="name-cell"><a href="%s" class="parent-dir">⬆️ Parent Directory</a></td>
            <td class="date-cell">-</td>
            <td class="size-cell">-</td>
            </tr>`, parent)
	}

	// Process directories first
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		encodedName := url.PathEscape(name)

		link := r.URL.Path
		if !strings.HasSuffix(link, "/") {
			link += "/"
		}
		link += encodedName + "/"

		info, err := entry.Info()
		modTime := "-"
		if err == nil {
			modTime = info.ModTime().Format("2006-01-02 15:04")
		}

		fmt.Fprintf(w, `<tr>
            <td class="name-cell"><a href="%s" class="dir-name">📁 %s/</a></td>
            <td class="date-cell">%s</td>
            <td class="size-cell">-</td>
            </tr>`, link, name, modTime)
	}

	// Process files
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		encodedName := url.PathEscape(name)

		link := r.URL.Path
		if !strings.HasSuffix(link, "/") {
			link += "/"
		}
		link += encodedName

		info, err := entry.Info()
		modTime, size := "-", "-"
		if err == nil {
			modTime = info.ModTime().Format("2006-01-02 15:04")
			if info.Size() < 1024 {
				size = fmt.Sprintf("%d B", info.Size())
			} else if info.Size() < 1024*1024 {
				size = fmt.Sprintf("%.1f KB", float64(info.Size())/1024)
			} else if info.Size() < 1024*1024*1024 {
				size = fmt.Sprintf("%.1f MB", float64(info.Size())/(1024*1024))
			} else {
				size = fmt.Sprintf("%.1f GB", float64(info.Size())/(1024*1024*1024))
			}
		}

		// Select icon based on file type
		icon := "📄"
		ext := strings.ToLower(filepath.Ext(name))
		switch {
		case ext == ".pdf":
			icon = "📕"
		case ext == ".zip", ext == ".rar", ext == ".7z", ext == ".tar", ext == ".gz":
			icon = "📦"
		case ext == ".jpg", ext == ".jpeg", ext == ".png", ext == ".gif", ext == ".webp":
			icon = "🖼️"
		case ext == ".mp3", ext == ".wav", ext == ".ogg":
			icon = "🎵"
		case ext == ".mp4", ext == ".avi", ext == ".mkv", ext == ".webm":
			icon = "🎬"
		}

		fmt.Fprintf(w, `<tr>
            <td class="name-cell"><a href="%s">%s %s</a></td>
            <td class="date-cell">%s</td>
            <td class="size-cell">%s</td>
            </tr>`, link, icon, name, modTime, size)
	}

	fmt.Fprintf(w, "<tr><th colspan=\"3\"><hr></th></tr>\n")
	fmt.Fprintf(w, "</table>\n")
	fmt.Fprintf(w, "<address>Go HTTP Server at %s Port %s</address>\n", r.Host, port)
	fmt.Fprintf(w, "</body></html>\n")
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

// Add gzip response writer wrapper
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// Check if file should be gzipped
func shouldGzip(path string) bool {
	// File types suitable for compression
	compressibleTypes := []string{
		".html", ".css", ".js", ".json", ".xml",
		".txt", ".md", ".svg", ".yaml", ".yml",
	}

	for _, ext := range compressibleTypes {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// Handle errors consistently
func handleError(w http.ResponseWriter, err error, statusCode int, logMsg string) {
	log.Printf("Error: %s - %v", logMsg, err)
	http.Error(w, http.StatusText(statusCode), statusCode)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// Add security headers
	addSecurityHeaders(w)

	// Check if client supports gzip
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		// Check if file type is suitable for gzip compression
		if shouldGzip(r.URL.Path) {
			gz := gzip.NewWriter(w)
			defer gz.Close()
			w.Header().Set("Content-Encoding", "gzip")
			w = &gzipResponseWriter{Writer: gz, ResponseWriter: w}
		}
	}

	// Declare support for range requests
	w.Header().Set("Accept-Ranges", "bytes")

	// Decode URL path, handle special characters
	filePath, err := url.PathUnescape(r.URL.Path)
	if err != nil {
		handleError(w, err, http.StatusBadRequest, "Bad request")
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
	// HTML and web files
	case strings.HasSuffix(filePath, ".html"), strings.HasSuffix(filePath, ".htm"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case strings.HasSuffix(filePath, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(filePath, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")

	// Images
	case strings.HasSuffix(filePath, ".png"):
		w.Header().Set("Content-Type", "image/png")
	case strings.HasSuffix(filePath, ".jpg"), strings.HasSuffix(filePath, ".jpeg"):
		w.Header().Set("Content-Type", "image/jpeg")
	case strings.HasSuffix(filePath, ".gif"):
		w.Header().Set("Content-Type", "image/gif")
	case strings.HasSuffix(filePath, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	case strings.HasSuffix(filePath, ".ico"):
		w.Header().Set("Content-Type", "image/x-icon")
	case strings.HasSuffix(filePath, ".webp"):
		w.Header().Set("Content-Type", "image/webp")

	// Documents
	case strings.HasSuffix(filePath, ".pdf"):
		w.Header().Set("Content-Type", "application/pdf")
	case strings.HasSuffix(filePath, ".doc"):
		w.Header().Set("Content-Type", "application/msword")
	case strings.HasSuffix(filePath, ".docx"):
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	case strings.HasSuffix(filePath, ".xls"):
		w.Header().Set("Content-Type", "application/vnd.ms-excel")
	case strings.HasSuffix(filePath, ".xlsx"):
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	case strings.HasSuffix(filePath, ".ppt"):
		w.Header().Set("Content-Type", "application/vnd.ms-powerpoint")
	case strings.HasSuffix(filePath, ".pptx"):
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")

	// Data formats
	case strings.HasSuffix(filePath, ".json"):
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case strings.HasSuffix(filePath, ".xml"):
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	case strings.HasSuffix(filePath, ".yaml"), strings.HasSuffix(filePath, ".yml"):
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	case strings.HasSuffix(filePath, ".csv"):
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")

	// Archives
	case strings.HasSuffix(filePath, ".zip"):
		w.Header().Set("Content-Type", "application/zip")
	case strings.HasSuffix(filePath, ".rar"):
		w.Header().Set("Content-Type", "application/vnd.rar")
	case strings.HasSuffix(filePath, ".7z"):
		w.Header().Set("Content-Type", "application/x-7z-compressed")
	case strings.HasSuffix(filePath, ".tar"):
		w.Header().Set("Content-Type", "application/x-tar")
	case strings.HasSuffix(filePath, ".gz"):
		w.Header().Set("Content-Type", "application/gzip")

	// Video formats
	case strings.HasSuffix(filePath, ".mp4"):
		w.Header().Set("Content-Type", "video/mp4")
	case strings.HasSuffix(filePath, ".webm"):
		w.Header().Set("Content-Type", "video/webm")
	case strings.HasSuffix(filePath, ".avi"):
		w.Header().Set("Content-Type", "video/x-msvideo")
	case strings.HasSuffix(filePath, ".mov"):
		w.Header().Set("Content-Type", "video/quicktime")
	case strings.HasSuffix(filePath, ".mkv"):
		w.Header().Set("Content-Type", "video/x-matroska")
	case strings.HasSuffix(filePath, ".m3u8"):
		w.Header().Set("Content-Type", "application/x-mpegURL")
	case strings.HasSuffix(filePath, ".ts"):
		w.Header().Set("Content-Type", "video/MP2T")

	// Audio formats
	case strings.HasSuffix(filePath, ".mp3"):
		w.Header().Set("Content-Type", "audio/mpeg")
	case strings.HasSuffix(filePath, ".wav"):
		w.Header().Set("Content-Type", "audio/wav")
	case strings.HasSuffix(filePath, ".ogg"):
		w.Header().Set("Content-Type", "audio/ogg")
	case strings.HasSuffix(filePath, ".m4a"):
		w.Header().Set("Content-Type", "audio/mp4")

	// Font files
	case strings.HasSuffix(filePath, ".ttf"):
		w.Header().Set("Content-Type", "font/ttf")
	case strings.HasSuffix(filePath, ".otf"):
		w.Header().Set("Content-Type", "font/otf")
	case strings.HasSuffix(filePath, ".woff"):
		w.Header().Set("Content-Type", "font/woff")
	case strings.HasSuffix(filePath, ".woff2"):
		w.Header().Set("Content-Type", "font/woff2")

	default:
		// If no matching MIME type is found, read first 512 bytes to auto-detect
		buffer := make([]byte, 512)
		_, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			handleError(w, err, http.StatusInternalServerError, "Internal Server Error")
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(buffer))
		file.Seek(0, 0) // Reset file pointer to beginning
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
