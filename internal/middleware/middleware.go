package middleware

import (
	"compress/gzip"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type gzipResponseWriter struct {
	gin.ResponseWriter
	writer *gzip.Writer
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	if code >= 100 && code < 200 || code == http.StatusNoContent || code == http.StatusNotModified {
		w.Header().Del("Content-Encoding")
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	w.Header().Del("Content-Length")
	return w.writer.Write(data)
}

func (w *gzipResponseWriter) WriteString(data string) (int, error) {
	w.Header().Del("Content-Length")
	return w.writer.Write([]byte(data))
}

func (w *gzipResponseWriter) Flush() {
	_ = w.writer.Flush()
	w.ResponseWriter.Flush()
}

func Gzip() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldCompressPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		appendVary(c.Writer.Header(), "Accept-Encoding")
		if c.Request.Method == http.MethodHead || !clientAcceptsGzip(c.Request) || c.Request.Header.Get("Range") != "" {
			c.Next()
			return
		}

		c.Header("Content-Encoding", "gzip")
		writer := gzip.NewWriter(c.Writer)
		original := c.Writer
		c.Writer = &gzipResponseWriter{ResponseWriter: original, writer: writer}
		defer func() {
			if recovered := recover(); recovered != nil {
				c.Writer = original
				original.Header().Del("Content-Encoding")
				panic(recovered)
			}
			status := original.Status()
			if status >= 100 && status < 200 || status == http.StatusNoContent || status == http.StatusNotModified {
				original.Header().Del("Content-Encoding")
				c.Writer = original
				return
			}
			_ = writer.Close()
			c.Writer = original
		}()
		c.Next()
	}
}

func clientAcceptsGzip(r *http.Request) bool {
	gzipQuality := -1.0
	wildcardQuality := -1.0
	for _, value := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		parts := strings.Split(strings.TrimSpace(value), ";")
		encoding := strings.ToLower(strings.TrimSpace(parts[0]))
		if encoding != "gzip" && encoding != "*" {
			continue
		}
		quality := 1.0
		for _, parameter := range parts[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && strings.EqualFold(key, "q") {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return false
				}
				if parsed < 0 || parsed > 1 {
					return false
				}
				quality = parsed
			}
		}
		if encoding == "gzip" {
			gzipQuality = quality
		} else {
			wildcardQuality = quality
		}
	}
	if gzipQuality >= 0 {
		return gzipQuality > 0
	}
	return wildcardQuality > 0
}

func shouldCompressPath(name string) bool {
	extension := strings.ToLower(path.Ext(name))
	if extension == "" {
		return true
	}
	switch extension {
	case ".css", ".csv", ".html", ".htm", ".js", ".json", ".map", ".md", ".svg", ".txt", ".xml":
		return true
	default:
		return false
	}
}

func appendVary(header http.Header, value string) {
	for _, existing := range header.Values("Vary") {
		for _, item := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(item), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}

func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.Writer.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; object-src 'none'; base-uri 'none'")
		if c.Request.TLS != nil {
			header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}
