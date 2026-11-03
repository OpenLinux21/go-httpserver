// Package logger implements a buffered log writer with a size-capped
// history file.
//
// Entries are mirrored to stdout in real time and appended to an in-memory
// buffer. The buffer is written back to the log file when it reaches a
// configured fill percentage, on a configured interval, and unconditionally
// on Close. The file keeps at most MaxEntries entries; when the cap is
// exceeded (plus a compaction margin), the oldest entries are dropped by an
// atomic rewrite, which bounds the file size regardless of traffic.
package logger

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const (
	minBufferSize     = 256 << 10
	minFlushPercent   = 50
	maxFlushPercent   = 95
	minMaxEntries     = 250
	maxMaxEntries     = 10_000_000
	maxFlushInterval  = time.Hour
	maxEntrySize      = 8 << 10 // single entries are truncated to this length
	defaultLogPath    = "latest.log"
	truncationMarker  = "...[truncated]"
	compactMinMargin  = 500
	compactPercentGap = 10 // margin is 10% of MaxEntries, at least compactMinMargin
)

// Config controls a BufferedLogger.
type Config struct {
	// BufferSize is the in-memory buffer capacity in bytes (minimum 256KiB).
	BufferSize int
	// FlushThreshold is the buffer fill percentage (50-95) that triggers a
	// write-back to the log file.
	FlushThreshold int
	// FlushInterval writes pending entries back at least this often.
	// Zero disables the periodic flush.
	FlushInterval time.Duration
	// MaxEntries caps the number of entries kept in the log file. When the
	// cap is exceeded, the oldest entries are discarded.
	MaxEntries int
	// Path is the log file location; empty means latest.log in the working
	// directory.
	Path string
	// Stdout, when non-nil, receives every entry immediately.
	Stdout io.Writer
}

// BufferedLogger is an io.Writer that buffers entries in memory and writes
// them back to a capped history file. It is safe for concurrent use.
type BufferedLogger struct {
	cfg    Config
	stdout io.Writer

	mu      sync.Mutex
	buf     []byte
	file    *os.File
	entries int
	closed  bool

	stop chan struct{}
	done chan struct{}
}

// New opens (or creates) the history file and returns a ready logger. A file
// left with a partially written last line by an unclean shutdown is repaired,
// and a file that already exceeds MaxEntries is compacted immediately.
func New(cfg Config) (*BufferedLogger, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("logger configuration: %w", err)
	}
	if cfg.Path == "" {
		cfg.Path = defaultLogPath
	}
	file, entries, err := openHistoryFile(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	l := &BufferedLogger{
		cfg:     cfg,
		stdout:  cfg.Stdout,
		buf:     make([]byte, 0, cfg.BufferSize),
		file:    file,
		entries: entries,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	if entries > cfg.MaxEntries {
		if err := l.compactLocked(); err != nil {
			file.Close()
			return nil, fmt.Errorf("compact log file: %w", err)
		}
	}
	if cfg.FlushInterval > 0 {
		go l.flushLoop()
	}
	return l, nil
}

func validateConfig(cfg Config) error {
	switch {
	case cfg.BufferSize < minBufferSize:
		return fmt.Errorf("buffer size must be at least %d bytes", minBufferSize)
	case cfg.FlushThreshold < minFlushPercent || cfg.FlushThreshold > maxFlushPercent:
		return fmt.Errorf("flush threshold must be between %d and %d percent", minFlushPercent, maxFlushPercent)
	case cfg.FlushInterval < 0 || cfg.FlushInterval > maxFlushInterval:
		return fmt.Errorf("flush interval must be between 0 (disabled) and %s", maxFlushInterval)
	case cfg.MaxEntries < minMaxEntries || cfg.MaxEntries > maxMaxEntries:
		return fmt.Errorf("max entries must be between %d and %d", minMaxEntries, maxMaxEntries)
	}
	return nil
}

// Write mirrors the entry to stdout (when configured) and appends it to the
// in-memory buffer, flushing to the file when a threshold is crossed.
func (l *BufferedLogger) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	data := prepareEntry(p)
	if l.stdout != nil {
		_, _ = l.stdout.Write(data) // real-time mirror, best effort
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return 0, errors.New("write to closed logger")
	}
	if len(l.buf)+len(data) > l.cfg.BufferSize {
		l.flushLocked()
	}
	l.buf = append(l.buf, data...)
	if len(l.buf) >= l.flushThresholdBytes() {
		l.flushLocked()
	}
	return len(p), nil
}

func (l *BufferedLogger) flushThresholdBytes() int {
	return l.cfg.BufferSize * l.cfg.FlushThreshold / 100
}

// Close flushes everything still buffered, syncs, and closes the file. It is
// idempotent and also stops the periodic flush goroutine.
func (l *BufferedLogger) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	l.flushLocked()
	var errs []error
	if err := l.file.Sync(); err != nil {
		errs = append(errs, err)
	}
	if err := l.file.Close(); err != nil {
		errs = append(errs, err)
	}
	l.mu.Unlock()

	if l.cfg.FlushInterval > 0 {
		close(l.stop)
		<-l.done
	}
	return errors.Join(errs...)
}

func (l *BufferedLogger) flushLoop() {
	ticker := time.NewTicker(l.cfg.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			close(l.done)
			return
		case <-ticker.C:
			l.mu.Lock()
			if !l.closed && len(l.buf) > 0 {
				l.flushLocked()
			}
			l.mu.Unlock()
		}
	}
}

// flushLocked writes the buffer to the file and resets it. It must be called
// with l.mu held. On write errors the buffer is discarded so memory stays
// bounded; the error is reported to stderr, which never routes through this
// logger.
func (l *BufferedLogger) flushLocked() {
	if len(l.buf) == 0 {
		return
	}
	if _, err := l.file.Write(l.buf); err != nil {
		fmt.Fprintf(os.Stderr, "logger: flush failed, discarding %d buffered bytes: %v\n", len(l.buf), err)
		l.buf = l.buf[:0]
		return
	}
	l.entries += bytes.Count(l.buf, newline)
	l.buf = l.buf[:0]
	if l.entries > l.cfg.MaxEntries+compactMargin(l.cfg.MaxEntries) {
		if err := l.compactLocked(); err != nil {
			fmt.Fprintf(os.Stderr, "logger: compact log file: %v\n", err)
		}
	}
}

func compactMargin(maxEntries int) int {
	margin := maxEntries * compactPercentGap / 100
	return max(compactMinMargin, margin)
}

// compactLocked rewrites the history file keeping only the newest MaxEntries
// entries. The rewrite goes through a temporary file and an atomic rename so
// a crash mid-compaction leaves the previous file intact.
func (l *BufferedLogger) compactLocked() error {
	source, err := os.Open(l.cfg.Path)
	if err != nil {
		return err
	}
	reader := bufio.NewReaderSize(source, 64<<10)

	tmpPath := l.cfg.Path + ".tmp"
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		source.Close()
		return err
	}

	// Skip the entries that exceed the cap. l.entries is authoritative, but
	// stop early on unexpected EOF so a drifted count cannot over-drop.
	drop := l.entries - l.cfg.MaxEntries
	for i := 0; i < drop; i++ {
		if _, err := reader.ReadBytes('\n'); err != nil {
			break
		}
	}

	kept := 0
	_, copyErr := l.copyCounting(tmp, reader, &kept)
	syncErr := tmp.Sync()
	closeErr := tmp.Close()
	source.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}

	if err := l.file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, l.cfg.Path); err != nil {
		l.reopenFile()
		return err
	}
	l.entries = kept
	return l.reopenFile()
}

func (l *BufferedLogger) copyCounting(dst io.Writer, src io.Reader, kept *int) (int64, error) {
	buf := make([]byte, 64<<10)
	var written int64
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			*kept += bytes.Count(buf[:n], newline)
			m, writeErr := dst.Write(buf[:n])
			written += int64(m)
			if writeErr != nil {
				return written, writeErr
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
}

func (l *BufferedLogger) reopenFile() error {
	file, err := os.OpenFile(l.cfg.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	l.file = file
	return nil
}

var newline = []byte{'\n'}

// prepareEntry copies p, truncates oversized entries, and guarantees a
// trailing newline so entry counting stays exact.
func prepareEntry(p []byte) []byte {
	data := p
	truncated := false
	if len(data) > maxEntrySize {
		data = data[:maxEntrySize]
		truncated = true
	}
	out := make([]byte, 0, len(data)+len(truncationMarker)+1)
	out = append(out, data...)
	if truncated {
		out = append(out, truncationMarker...)
	}
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out
}

// openHistoryFile repairs a partial trailing line from an unclean shutdown,
// counts the existing entries, and returns the file opened for appending.
func openHistoryFile(path string) (*os.File, int, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, 0, err
	}
	if info.Size() > 0 {
		last := make([]byte, 1)
		if _, err := file.ReadAt(last, info.Size()-1); err != nil {
			file.Close()
			return nil, 0, err
		}
		if last[0] != '\n' {
			if _, err := file.WriteAt(newline, info.Size()); err != nil {
				file.Close()
				return nil, 0, err
			}
		}
	}
	file.Close()

	entries, err := countLines(path)
	if err != nil {
		return nil, 0, err
	}
	appendFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, 0, err
	}
	return appendFile, entries, nil
}

func countLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	count := 0
	buf := make([]byte, 128<<10)
	for {
		n, readErr := file.Read(buf)
		count += bytes.Count(buf[:n], newline)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return count, nil
			}
			return 0, readErr
		}
	}
}
