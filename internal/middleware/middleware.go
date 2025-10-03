package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type GzipResponseWriter struct {
	io.Writer
	gin.ResponseWriter
}

func (w *GzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (w *GzipResponseWriter) WriteString(s string) (int, error) {
	return w.Writer.Write([]byte(s))
}

func ShouldGzip(path string) bool {
	// Check if path contains an extension
	lastDot := strings.LastIndex(path, ".")
	if lastDot == -1 {
		return false
	}

	ext := strings.ToLower(path[lastDot:])
	return ext == ".html" || ext == ".css" || ext == ".js" || ext == ".json" || ext == ".xml" || ext == ".txt"
}

func AddSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	// HSTS should only be set by caller when HTTPS is enabled. Keep here but comment.
	// w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'")
}

func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ShouldGzip(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gz := gzip.NewWriter(w)
		defer gz.Close()

		w.Header().Set("Content-Encoding", "gzip")
		// Use a http-specific gzip writer wrapper
		next.ServeHTTP(&HTTPGzipResponseWriter{Writer: gz, ResponseWriter: w}, r)
	})
}

// ClientAcceptsGzip checks request headers for gzip support
func ClientAcceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

// GzipGinMiddleware writes gzipped response for gin context; it assumes caller checked ShouldGzip and Accept-Encoding
func GzipGinMiddleware(c *gin.Context) {
	gz := gzip.NewWriter(c.Writer)
	defer gz.Close()

	c.Header("Content-Encoding", "gzip")
	// hijack response writer by replacing c.Writer
	gw := &GzipResponseWriter{Writer: gz, ResponseWriter: c.Writer}
	c.Writer = gw
	c.Next()
}

// HTTPGzipResponseWriter is used for net/http handlers
type HTTPGzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *HTTPGzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// SetHSTS sets the Strict-Transport-Security header for HTTPS responses.
// Should only be called when the server is serving over HTTPS.
func SetHSTS(w http.ResponseWriter) {
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
}
