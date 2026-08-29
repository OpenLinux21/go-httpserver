package logger

import (
	"fmt"
	"io"
	"os"
)

func SetupGinLogger() (io.Writer, io.Closer, error) {
	file, err := os.OpenFile("latest.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	return io.MultiWriter(os.Stdout, file), file, nil
}
