package handlers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenLinux21/go-httpserver/internal/config"
	"github.com/gin-gonic/gin"
)

func TestHandlerHTTPBehavior(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "hello.txt"), "hello")
	mustWriteFile(t, filepath.Join(root, "404.html"), "custom missing")
	if err := os.Mkdir(filepath.Join(root, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(root, "files", "<img onerror=alert(1)>.txt"), "unsafe name")

	router, closeHandler := newTestRouter(t, baseTestConfig(), root)
	defer closeHandler()

	tests := []struct {
		name       string
		method     string
		path       string
		status     int
		contains   string
		notContain string
	}{
		{name: "file", method: http.MethodGet, path: "/hello.txt", status: http.StatusOK, contains: "hello"},
		{name: "head", method: http.MethodHead, path: "/hello.txt", status: http.StatusOK, notContain: "hello"},
		{name: "custom 404", method: http.MethodGet, path: "/missing", status: http.StatusNotFound, contains: "custom missing"},
		{name: "method", method: http.MethodPost, path: "/hello.txt", status: http.StatusMethodNotAllowed},
		{name: "directory redirect", method: http.MethodGet, path: "/files", status: http.StatusMovedPermanently, contains: "/files/"},
		{name: "escaped listing", method: http.MethodGet, path: "/files/", status: http.StatusOK, contains: "&lt;img", notContain: "<img onerror"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%q", response.Code, test.status, response.Body.String())
			}
			if test.contains != "" && !strings.Contains(response.Body.String(), test.contains) && !strings.Contains(response.Header().Get("Location"), test.contains) {
				t.Errorf("response does not contain %q: body=%q headers=%v", test.contains, response.Body.String(), response.Header())
			}
			if test.notContain != "" && strings.Contains(response.Body.String(), test.notContain) {
				t.Errorf("response unexpectedly contains %q", test.notContain)
			}
			if test.name == "method" && response.Header().Get("Allow") != "GET, HEAD" {
				t.Errorf("Allow = %q", response.Header().Get("Allow"))
			}
		})
	}
}

func TestHandlerConfinesSymlinksToRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "404.html"), "missing")
	mustWriteFile(t, filepath.Join(outside, "secret.txt"), "top secret")
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	router, closeHandler := newTestRouter(t, baseTestConfig(), root)
	defer closeHandler()
	request := httptest.NewRequest(http.MethodGet, "/outside/secret.txt", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code == http.StatusOK || strings.Contains(response.Body.String(), "top secret") {
		t.Fatalf("root escape succeeded: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestHTTPSRedirectWhenEnabled(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "hello.txt"), "hello")

	cfg := baseTestConfig()
	cfg.EnableHTTPS = true
	cfg.HTTPSPort = "8443"
	router, closeHandler := newTestRouter(t, cfg, root)
	defer closeHandler()

	t.Run("plaintext redirects preserving path and query", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/hello.txt?a=1", nil)
		request.Host = "example.com"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusPermanentRedirect {
			t.Fatalf("status = %d, want 308", response.Code)
		}
		if location := response.Header().Get("Location"); location != "https://example.com:8443/hello.txt?a=1" {
			t.Fatalf("Location = %q", location)
		}
	})

	t.Run("https port with port-bearing host replaces the port", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "example.com:8081"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if location := response.Header().Get("Location"); location != "https://example.com:8443/" {
			t.Fatalf("Location = %q", location)
		}
	})

	t.Run("tls requests are served normally", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/hello.txt", nil)
		request.TLS = &tls.ConnectionState{}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.String() != "hello" {
			t.Fatalf("unexpected response: status=%d body=%q", response.Code, response.Body.String())
		}
	})

	t.Run("disabled by default", func(t *testing.T) {
		plainRouter, closePlain := newTestRouter(t, baseTestConfig(), root)
		defer closePlain()
		request := httptest.NewRequest(http.MethodGet, "/hello.txt", nil)
		response := httptest.NewRecorder()
		plainRouter.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 when https is disabled", response.Code)
		}
	})
}

func newTestRouter(t *testing.T, cfg config.Config, root string) (*gin.Engine, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg.RootDirectory = root
	handler, err := New(cfg)
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}
	router := gin.New()
	router.NoRoute(handler.Serve)
	return router, func() { _ = handler.Close() }
}

func baseTestConfig() config.Config {
	return config.Config{
		IndexFiles:    []string{"index.html"},
		NotFoundPage:  "404.html",
		ForbiddenPage: "403.html",
	}
}

func mustWriteFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
