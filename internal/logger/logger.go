package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/OpenLinux21/go-httpserver/internal/utils"
)

// LogFile is the opened log file used for request logging. It is opened once at startup.
var LogFile *os.File

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

// SetupGinLogger opens the log file once and returns an io.Writer suitable for gin's logger.
func SetupGinLogger() (io.Writer, error) {
	// Create or open log file
	lf, err := os.OpenFile("latest.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}
	LogFile = lf

	// Create a writer that writes to both console and file
	return NewMultiWriter(os.Stdout, LogFile), nil
}

// CloseLogFile closes the global log file if opened
func CloseLogFile() {
	if LogFile != nil {
		_ = LogFile.Close()
		LogFile = nil
	}
}

// LogRequestDetails logs request details to latest.log
// clientIP should be extracted correctly (e.g. using gin.Context.ClientIP()).
func LogRequestDetails(clientIP string, filePath string, bytesSent int64, port string) {
	requestTime := time.Now().Format("2006-01-02 15:04:05")
	randomString := utils.GenerateRandomString(16)
	logDetails := fmt.Sprintf("%s | ClientIP: %s | Port: %s | File: %s | Time: %s | BytesSent: %d\n",
		randomString, clientIP, port, filePath, requestTime, bytesSent)

	if LogFile == nil {
		// Fallback: write directly to stdout if log file not ready
		log.Printf("%s", strings.TrimSpace(logDetails))
		return
	}

	if _, err := io.WriteString(LogFile, logDetails); err != nil {
		log.Printf("Error writing to log file: %v", err)
	}
}
