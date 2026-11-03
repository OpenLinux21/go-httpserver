package logger

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testConfig(t *testing.T, path string) Config {
	t.Helper()
	return Config{
		BufferSize:     256 << 10,
		FlushThreshold: 50,
		FlushInterval:  0,
		MaxEntries:     250,
		Path:           path,
		Stdout:         io.Discard,
	}
}

func writeLines(t *testing.T, w io.Writer, count, size int) {
	t.Helper()
	line := strings.Repeat("x", size-1) + "\n"
	for i := 0; i < count; i++ {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatalf("write line %d: %v", i, err)
		}
	}
}

func logLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	text := string(data)
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	base := testConfig(t, filepath.Join(t.TempDir(), "latest.log"))
	tests := []func(Config) Config{
		func(c Config) Config { c.BufferSize = 256<<10 - 1; return c },
		func(c Config) Config { c.FlushThreshold = 49; return c },
		func(c Config) Config { c.FlushThreshold = 96; return c },
		func(c Config) Config { c.FlushInterval = -time.Second; return c },
		func(c Config) Config { c.MaxEntries = 249; return c },
	}
	for i, mutate := range tests {
		cfg := mutate(base)
		if _, err := New(cfg); err == nil {
			t.Errorf("case %d: New unexpectedly accepted %+v", i, cfg)
		}
	}
}

func TestFlushesOnCloseOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.log")
	l, err := New(testConfig(t, path))
	if err != nil {
		t.Fatal(err)
	}
	writeLines(t, l, 100, 64)
	if got := len(logLines(t, path)); got != 0 {
		t.Fatalf("file has %d lines before flush, want 0", got)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if got := len(logLines(t, path)); got != 100 {
		t.Fatalf("file has %d lines after close, want 100", got)
	}
	// Close must be idempotent.
	if err := l.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if _, err := l.Write([]byte("after close\n")); err == nil {
		t.Fatal("write after close unexpectedly succeeded")
	}
}

func TestFlushesAtThreshold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.log")
	cfg := testConfig(t, path) // 256KiB buffer, flush at 50% = 128KiB
	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	writeLines(t, l, 40, 4096) // 160KiB > 128KiB
	if got := len(logLines(t, path)); got == 0 {
		t.Fatal("threshold flush did not write anything to the file")
	}
}

func TestCompactsToMaxEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.log")
	cfg := testConfig(t, path) // MaxEntries 250, margin max(500, 25) = 500
	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeLines(t, l, 800, 64) // 800 > 250+500 triggers one compaction
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	lines := logLines(t, path)
	if len(lines) != 250 {
		t.Fatalf("file has %d lines, want 250", len(lines))
	}
	if !strings.HasPrefix(lines[len(lines)-1], "xxx") {
		t.Fatalf("unexpected last entry %q", lines[len(lines)-1])
	}
}

func TestRestartsCountAndKeepsEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.log")
	cfg := testConfig(t, path)

	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Write([]byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	l2, err := New(cfg) // must pick up the existing entry count
	if err != nil {
		t.Fatal(err)
	}
	if l2.entries != 1 {
		t.Fatalf("entries after restart = %d, want 1", l2.entries)
	}
	writeLines(t, l2, 14, 32)
	if err := l2.Close(); err != nil {
		t.Fatal(err)
	}
	if got := len(logLines(t, path)); got != 15 {
		t.Fatalf("file has %d lines, want 15", got)
	}
}

func TestRepairsPartialTrailingLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.log")
	if err := os.WriteFile(path, []byte("complete line\npartial-without-newline"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t, path)
	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if l.entries != 2 {
		t.Fatalf("entries = %d, want 2 (repaired partial counts as one)", l.entries)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	lines := logLines(t, path)
	if len(lines) != 2 || lines[1] != "partial-without-newline" {
		t.Fatalf("unexpected lines %q", lines)
	}
}

func TestCompactsOnStartupWhenOverCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.log")
	var content bytes.Buffer
	for i := 0; i < 300; i++ {
		content.WriteString("old\n")
	}
	if err := os.WriteFile(path, content.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t, path) // cap 250
	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if got := len(logLines(t, path)); got != 250 {
		t.Fatalf("file has %d lines after startup compaction, want 250", got)
	}
}

func TestFlushIntervalWritesBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.log")
	cfg := testConfig(t, path)
	cfg.FlushInterval = 50 * time.Millisecond
	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, err := l.Write([]byte("periodic\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(logLines(t, path)) == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("periodic flush never wrote the entry")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestTruncatesOversizedEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.log")
	cfg := testConfig(t, path)
	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("y", 20<<10)
	if _, err := l.Write([]byte(huge)); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	lines := logLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(lines))
	}
	if !strings.HasSuffix(lines[0], truncationMarker) || len(lines[0]) > maxEntrySize+len(truncationMarker) {
		t.Fatalf("entry not truncated as expected: %d bytes, suffix %q", len(lines[0]), lines[0][len(lines[0])-30:])
	}
}

func TestMirrorsToStdoutInRealTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.log")
	cfg := testConfig(t, path)
	stdout := &bytes.Buffer{}
	cfg.Stdout = stdout
	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, err := l.Write([]byte("mirror me")); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "mirror me\n" {
		t.Fatalf("stdout = %q, want %q", got, "mirror me\n")
	}
}

func TestConcurrentWritesAreCountedExactly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.log")
	cfg := testConfig(t, path)
	cfg.MaxEntries = 10_000 // keep every entry; this test writes 8000
	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for g := 0; g < 8; g++ {
		group.Add(1)
		go func() {
			defer group.Done()
			writeLines(t, l, 1000, 64)
		}()
	}
	group.Wait()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if got := len(logLines(t, path)); got != 8000 {
		t.Fatalf("file has %d lines, want 8000", got)
	}
}
