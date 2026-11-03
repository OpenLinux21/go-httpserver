package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestGzipNegotiation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Gzip())
	handler := func(c *gin.Context) { c.String(http.StatusOK, "compress me") }
	router.GET("/file.txt", handler)
	router.HEAD("/file.txt", handler)

	t.Run("compresses accepted response", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		request.Header.Set("Accept-Encoding", "br, gzip")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Header().Get("Content-Encoding") != "gzip" {
			t.Fatalf("Content-Encoding = %q", response.Header().Get("Content-Encoding"))
		}
		reader, err := gzip.NewReader(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "compress me" {
			t.Fatalf("body = %q", body)
		}
	})

	t.Run("respects zero quality", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		request.Header.Set("Accept-Encoding", "gzip;q=0")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Header().Get("Content-Encoding") != "" || response.Body.String() != "compress me" {
			t.Fatalf("unexpected response: headers=%v body=%q", response.Header(), response.Body.String())
		}
	})

	t.Run("explicit refusal overrides wildcard", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		request.Header.Set("Accept-Encoding", "*;q=1, GZIP;q=0")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Header().Get("Content-Encoding") != "" {
			t.Fatalf("Content-Encoding = %q", response.Header().Get("Content-Encoding"))
		}
	})

	t.Run("skips ranges", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		request.Header.Set("Accept-Encoding", "gzip")
		request.Header.Set("Range", "bytes=0-1")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Header().Get("Content-Encoding") != "" {
			t.Fatalf("Content-Encoding = %q", response.Header().Get("Content-Encoding"))
		}
	})

	t.Run("skips head responses", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodHead, "/file.txt", nil)
		request.Header.Set("Accept-Encoding", "gzip")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Header().Get("Content-Encoding") != "" {
			t.Fatalf("unexpected response: headers=%v body=%q", response.Header(), response.Body.String())
		}
	})
}

func TestGzipRestoresWriterAfterPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(Gzip())
	router.GET("/panic.txt", func(*gin.Context) { panic("boom") })

	request := httptest.NewRequest(http.MethodGet, "/panic.txt", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding = %q", response.Header().Get("Content-Encoding"))
	}
}

func TestGzipPoolReuseAcrossRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Gzip())
	router.GET("/file.txt", func(c *gin.Context) { c.String(http.StatusOK, "compress me %s", strings.Repeat("x", 512)) })

	var group sync.WaitGroup
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			request := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
			request.Header.Set("Accept-Encoding", "gzip")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			reader, err := gzip.NewReader(response.Body)
			if err != nil {
				t.Errorf("gzip.NewReader: %v", err)
				return
			}
			body, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("decompress pooled response: %v", err)
				return
			}
			if !strings.HasPrefix(string(body), "compress me ") || len(body) != len("compress me ")+512 {
				t.Errorf("unexpected body length %d", len(body))
			}
		}()
	}
	group.Wait()
}

func TestRateLimiterBansAndExpiresClients(t *testing.T) {
	limiter := NewRateLimiter()
	now := time.Now()
	for i := 0; i < burstPerSecondThreshold-1; i++ {
		_, _, banned, newly := limiter.evaluate("192.0.2.1", now)
		if banned || newly {
			t.Fatalf("request %d banned too early", i+1)
		}
	}
	_, until, banned, newly := limiter.evaluate("192.0.2.1", now)
	if !banned || !newly || until.Sub(now) != initialBanDuration {
		t.Fatalf("ban = %v until=%v newly=%v", banned, until, newly)
	}
	// A request that is rejected by an existing ban is not "new".
	if _, _, _, newly = limiter.evaluate("192.0.2.1", now.Add(time.Second)); newly {
		t.Fatal("existing ban reported as newly banned")
	}

	// Stale entries are evicted by the periodic cleanup of their shard.
	shard := limiter.shardFor("new")
	shard.clients["stale"] = &clientState{lastSeen: now.Add(-clientStateTTL), banDuration: initialBanDuration}
	shard.lastCleanup = now.Add(-cleanupInterval)
	limiter.evaluate("new", now)
	if _, exists := shard.clients["stale"]; exists {
		t.Fatal("stale client was not removed")
	}
}

func TestRateLimiterBoundsTrackedClients(t *testing.T) {
	limiter := NewRateLimiter()
	now := time.Now()
	for i := 0; i < maxTrackedClients/shardCount+512; i++ {
		limiter.evaluate("192.0.2."+strconv.Itoa(1+i%250)+"."+strconv.Itoa(i/250%250), now)
	}
	total := 0
	for i := range limiter.shards {
		total += len(limiter.shards[i].clients)
	}
	if total > maxTrackedClients {
		t.Fatalf("tracked clients = %d, cap is %d", total, maxTrackedClients)
	}
}

func TestRateLimiterReturns429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewRateLimiter()
	shard := limiter.shardFor("192.0.2.1")
	shard.clients["192.0.2.1"] = &clientState{
		lastSeen:    time.Now(),
		banUntil:    time.Now().Add(time.Minute),
		banDuration: initialBanDuration,
	}
	router := gin.New()
	router.SetTrustedProxies(nil)
	router.Use(limiter.Middleware())
	router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" || !strings.Contains(response.Body.String(), "too many requests") {
		t.Fatalf("unexpected response: status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
}
