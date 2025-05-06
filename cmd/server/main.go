package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OpenLinux21/go-httpserver/internal/config"
	"github.com/OpenLinux21/go-httpserver/internal/handlers"
	"github.com/OpenLinux21/go-httpserver/internal/logger"
	"github.com/OpenLinux21/go-httpserver/internal/middleware"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	if err := config.LoadConfig(); err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Setup logger
	logWriter, err := logger.SetupGinLogger()
	if err != nil {
		log.Fatalf("Error setting up logger: %v", err)
	}
	gin.DefaultWriter = logWriter

	// Create Gin router
	router := gin.New()

	// Use Gin's logger and recovery middleware
	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("[%s] | %s | %d | %s | %s | %s | %s | %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			param.ClientIP,
			param.StatusCode,
			param.Method,
			param.Path,
			param.Request.UserAgent(),
			param.Latency,
			param.ErrorMessage,
		)
	}))
	router.Use(gin.Recovery())

	// Add our custom middleware
	router.Use(func(c *gin.Context) {
		middleware.GzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlers.HandleRequest(w, r)
		})).ServeHTTP(c.Writer, c.Request)
	})

	// Create HTTP server
	server := &http.Server{
		Addr:    config.GlobalConfig.IPAddress + ":" + config.GlobalConfig.Port,
		Handler: router,
	}

	// Create HTTPS server if enabled
	var httpsServer *http.Server
	if config.GlobalConfig.EnableHTTPS {
		httpsServer = &http.Server{
			Addr:    config.GlobalConfig.IPAddress + ":" + config.GlobalConfig.HTTPSPort,
			Handler: router,
		}
	}

	// Start HTTP server
	go func() {
		log.Printf("Starting HTTP server on %s\n", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting HTTP server: %v", err)
		}
	}()

	// Start HTTPS server if enabled
	if httpsServer != nil {
		go func() {
			log.Printf("Starting HTTPS server on %s\n", httpsServer.Addr)
			if err := httpsServer.ListenAndServeTLS(config.GlobalConfig.CertFile, config.GlobalConfig.KeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Error starting HTTPS server: %v", err)
			}
		}()
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Create shutdown context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown servers
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Error shutting down HTTP server: %v", err)
	}
	if httpsServer != nil {
		if err := httpsServer.Shutdown(ctx); err != nil {
			log.Fatalf("Error shutting down HTTPS server: %v", err)
		}
	}

	log.Println("Server shutdown complete")
}
