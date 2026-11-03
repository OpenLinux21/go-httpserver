package config

import (
	"os"
	"strings"
	"testing"
)

func TestParseUsesDefaults(t *testing.T) {
	cfg, err := parse(strings.NewReader("port = 9090\n"))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.Port != "9090" || cfg.RootDirectory != "./website" || cfg.EnableHTTPS {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.LogBufferSize != 1<<20 || cfg.LogFlushThreshold != 75 || cfg.LogFlushInterval != 5 || cfg.LogMaxEntries != 12500 {
		t.Fatalf("unexpected logging defaults: %+v", cfg)
	}
}

func TestParseLoggingOptions(t *testing.T) {
	cfg, err := parse(strings.NewReader(strings.Join([]string{
		"log-buffer-size = 512KiB",
		"log-flush-threshold = 95",
		"log-flush-interval = 0",
		"log-max-entries = 1000",
		"log-buffer-size = 1048576",
	}, "\n")))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.LogBufferSize != 1<<20 {
		t.Fatalf("LogBufferSize = %d, want %d", cfg.LogBufferSize, 1<<20)
	}
	if cfg.LogFlushThreshold != 95 || cfg.LogFlushInterval != 0 || cfg.LogMaxEntries != 1000 {
		t.Fatalf("unexpected logging config: %+v", cfg)
	}
}

func TestParseByteSize(t *testing.T) {
	valid := map[string]int64{
		"1048576": 1 << 20,
		"1MiB":    1 << 20,
		"1 mib":   1 << 20,
		"256kib":  256 << 10,
		"1GiB":    1 << 30,
		"512B":    512,
	}
	for input, want := range valid {
		got, err := parseByteSize(input)
		if err != nil || got != want {
			t.Errorf("parseByteSize(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []string{"", "KiB", "-5MiB", "1TiB", "1.5MiB", "0"} {
		if _, err := parseByteSize(input); err == nil {
			t.Errorf("parseByteSize(%q) unexpectedly succeeded", input)
		}
	}
}

func TestValidateRejectsInvalidLoggingConfig(t *testing.T) {
	base := func() Config {
		cfg := defaults()
		cfg.RootDirectory = t.TempDir()
		return cfg
	}
	tests := []func(Config) Config{
		func(c Config) Config { c.LogBufferSize = 256<<10 - 1; return c },
		func(c Config) Config { c.LogBufferSize = 128 << 20; return c },
		func(c Config) Config { c.LogFlushThreshold = 49; return c },
		func(c Config) Config { c.LogFlushThreshold = 96; return c },
		func(c Config) Config { c.LogFlushInterval = -1; return c },
		func(c Config) Config { c.LogFlushInterval = 3601; return c },
		func(c Config) Config { c.LogMaxEntries = 249; return c },
		func(c Config) Config { c.LogMaxEntries = 20_000_000; return c },
	}
	for i, mutate := range tests {
		cfg := mutate(base())
		if err := cfg.Validate(); err == nil {
			t.Errorf("case %d: Validate unexpectedly accepted %+v", i, cfg)
		}
	}
	// Boundary values must stay accepted, including a disabled interval.
	cfg := base()
	cfg.LogBufferSize = 256 << 10
	cfg.LogFlushThreshold = 50
	cfg.LogFlushInterval = 0
	cfg.LogMaxEntries = 250
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate rejected boundary logging config: %v", err)
	}
}

func TestParseRejectsInvalidValues(t *testing.T) {
	tests := []string{
		"enable-https = perhaps\n",
		"unknown = value\n",
		"missing separator\n",
	}
	for _, input := range tests {
		if _, err := parse(strings.NewReader(input)); err == nil {
			t.Errorf("parse(%q) unexpectedly succeeded", input)
		}
	}
}

func TestValidateRejectsPathsOutsideRoot(t *testing.T) {
	cfg := defaults()
	cfg.RootDirectory = t.TempDir()
	cfg.NotFoundPage = "../404.html"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate unexpectedly accepted an escaping error page")
	}
}

func TestValidateRejectsInvalidPortAndProxy(t *testing.T) {
	cfg := defaults()
	cfg.RootDirectory = t.TempDir()
	cfg.Port = "70000"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate unexpectedly accepted an invalid port")
	}

	cfg.Port = "8080"
	cfg.TrustedProxies = []string{"not-a-proxy"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate unexpectedly accepted an invalid proxy")
	}
}

func TestValidateRejectsEquivalentHTTPAndHTTPSPorts(t *testing.T) {
	root := t.TempDir()
	cert := root + "/cert.pem"
	key := root + "/key.pem"
	if err := os.WriteFile(cert, []byte("certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaults()
	cfg.RootDirectory = root
	cfg.EnableHTTPS = true
	cfg.CertFile = cert
	cfg.KeyFile = key
	cfg.Port = "8080"
	cfg.HTTPSPort = "08080"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate unexpectedly accepted equivalent ports")
	}
}
