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
