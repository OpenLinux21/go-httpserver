package logger

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/OpenLinux21/go-httpserver/internal/utils"
)

// LogRequestDetails logs request details to latest.log
func LogRequestDetails(r *http.Request, filePath string, bytesSent int64, port string) {
	clientIP := strings.Split(r.RemoteAddr, ":")[0]
	requestTime := time.Now().Format("2006-01-02 15:04:05")
	randomString := utils.GenerateRandomString(16)
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

// MultiWriter creates a writer that writes to multiple targets
type MultiWriter struct {
	writers []io.Writer
}

// NewMultiWriter creates a new MultiWriter
func NewMultiWriter(writers ...io.Writer) *MultiWriter {
	return &MultiWriter{writers: writers}
}

// Write implements the io.Writer interface
func (t *MultiWriter) Write(p []byte) (n int, err error) {
	for _, w := range t.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}
		if n != len(p) {
			err = io.ErrShortWrite
			return
		}
	}
	return len(p), nil
}

// SetupGinLogger sets up GIN's logging output
func SetupGinLogger() (io.Writer, error) {
	// Create or open log file
	logFile, err := os.OpenFile("latest.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}

	// Create a writer that writes to both console and file
	return NewMultiWriter(os.Stdout, logFile), nil
}
