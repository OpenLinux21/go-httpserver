package handlers

import (
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

	router, closeHandler := newTestRouter(t, root)
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

	router, closeHandler := newTestRouter(t, root)
	defer closeHandler()
	request := httptest.NewRequest(http.MethodGet, "/outside/secret.txt", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code == http.StatusOK || strings.Contains(response.Body.String(), "top secret") {
		t.Fatalf("root escape succeeded: status=%d body=%q", response.Code, response.Body.String())
	}
}

func newTestRouter(t *testing.T, root string) (*gin.Engine, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler, err := New(config.Config{
		RootDirectory: root,
		IndexFiles:    []string{"index.html"},
		NotFoundPage:  "404.html",
		ForbiddenPage: "403.html",
	})
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}
	router := gin.New()
	router.NoRoute(handler.Serve)
	return router, func() { _ = handler.Close() }
}

func mustWriteFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
