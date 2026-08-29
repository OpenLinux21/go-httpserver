package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/OpenLinux21/go-httpserver/internal/config"
	"github.com/OpenLinux21/go-httpserver/internal/handlers"
	"github.com/OpenLinux21/go-httpserver/internal/logger"
	"github.com/OpenLinux21/go-httpserver/internal/middleware"
	"github.com/gin-gonic/gin"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 5 * time.Minute
	idleTimeout       = 90 * time.Second
	shutdownTimeout   = 10 * time.Second
	maxHeaderBytes    = 1 << 20
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logWriter, logCloser, err := logger.SetupGinLogger()
	if err != nil {
		return fmt.Errorf("set up logger: %w", err)
	}
	defer logCloser.Close()
	gin.DefaultWriter = logWriter

	fileHandler, err := handlers.New(cfg)
	if err != nil {
		return err
	}
	defer fileHandler.Close()

	router := gin.New()
	if err := router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return fmt.Errorf("configure trusted proxies: %w", err)
	}
	router.Use(gin.LoggerWithFormatter(accessLogFormatter))
	router.Use(gin.Recovery())
	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.NewRateLimiter().Middleware())
	router.Use(middleware.Gzip())
	router.NoRoute(fileHandler.Serve)
	router.NoMethod(fileHandler.Serve)

	servers := []*http.Server{newServer(net.JoinHostPort(cfg.IPAddress, cfg.Port), router)}
	if cfg.EnableHTTPS {
		servers = append(servers, newServer(net.JoinHostPort(cfg.IPAddress, cfg.HTTPSPort), router))
	}

	serveErrors := make(chan error, len(servers))
	for i, server := range servers {
		server, tlsEnabled := server, i > 0
		go func() {
			protocol := "HTTP"
			var serveErr error
			if tlsEnabled {
				protocol = "HTTPS"
				log.Printf("Starting HTTPS server on %s", server.Addr)
				serveErr = server.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
			} else {
				log.Printf("Starting HTTP server on %s", server.Addr)
				serveErr = server.ListenAndServe()
			}
			if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				serveErrors <- fmt.Errorf("%s server: %w", protocol, serveErr)
			}
		}()
	}

	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var runErr error
	select {
	case <-signalContext.Done():
		log.Println("Shutdown signal received")
	case runErr = <-serveErrors:
		log.Printf("Server stopped unexpectedly: %v", runErr)
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := shutdownServers(shutdownContext, servers); err != nil && runErr == nil {
		runErr = err
	}
	if runErr == nil {
		log.Println("Server shutdown complete")
	}
	return runErr
}

func newServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
}

func shutdownServers(ctx context.Context, servers []*http.Server) error {
	errorsChannel := make(chan error, len(servers))
	var waitGroup sync.WaitGroup
	for _, server := range servers {
		waitGroup.Add(1)
		go func(server *http.Server) {
			defer waitGroup.Done()
			if err := server.Shutdown(ctx); err != nil {
				errorsChannel <- fmt.Errorf("shut down %s: %w", server.Addr, err)
				_ = server.Close()
			}
		}(server)
	}
	waitGroup.Wait()
	close(errorsChannel)

	var result error
	for err := range errorsChannel {
		result = errors.Join(result, err)
	}
	return result
}

func accessLogFormatter(param gin.LogFormatterParams) string {
	return fmt.Sprintf("[%s] | %s | %d | %s | %q | %q | %s | %q\n",
		param.TimeStamp.Format("2006/01/02 - 15:04:05"),
		param.ClientIP,
		param.StatusCode,
		param.Method,
		param.Path,
		param.Request.UserAgent(),
		param.Latency,
		param.ErrorMessage,
	)
}
