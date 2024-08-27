package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const randomStringCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

var (
	configPath    = "config.conf"
	ipAddress     string
	port          string
	rootDirectory string
	indexFiles    []string
	notFoundPage  string
	forbiddenPage string
)

func loadConfig() {
	// 读取配置文件
	content, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("读取配置文件错误: %v", err)
	}

	// 解析配置内容
	lines := strings.Split(string(content), "\n")
	for lineNumber, line := range lines {
		line = strings.TrimSpace(line) // 去除行首尾的空白字符

		// 跳过空行和注释行
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			log.Fatalf("无效的配置行（行 %d）: %s", lineNumber+1, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "ip-address":
			ipAddress = value
		case "port":
			port = value
		case "root":
			rootDirectory = value
		case "index":
			indexFiles = strings.Split(value, ";")
		case "404-error":
			notFoundPage = value
		case "403-error":
			forbiddenPage = value
		default:
			log.Printf("警告: 未知配置项（行 %d）: %s", lineNumber+1, line)
		}
	}
}

func generateRandomString(length int) string {
	// 生成指定長度的隨機字符串
	rand.Seed(time.Now().UnixNano())
	randomBytes := make([]byte, length)
	for i := range randomBytes {
		randomBytes[i] = randomStringCharset[rand.Intn(len(randomStringCharset))]
	}
	return string(randomBytes)
}

func logRequestDetails(r *http.Request, filePath string, bytesSent int64) {
	// 記錄請求詳細信息，包括隨機字符串
	clientIP := strings.Split(r.RemoteAddr, ":")[0]
	requestTime := time.Now().Format("2006-01-02 15:04:05")
	randomString := generateRandomString(16)
	logDetails := fmt.Sprintf("%s | ClientIP: %s | Port: %s | File: %s | Time: %s | BytesSent: %d\n", randomString, clientIP, port, filePath, requestTime, bytesSent)

	// 打印到控制台
	fmt.Print(logDetails)

	// 寫入日誌文件
	logFile, err := os.OpenFile("latest.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("打開日誌文件錯誤: %v", err)
		return
	}
	defer logFile.Close()

	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.Print(logDetails)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// 處理請求
	filePath := r.URL.Path
	if filePath == "/" {
		// 如果請求的是根目錄，嘗試使用預設的索引文件
		for _, indexFile := range indexFiles {
			if _, err := os.Stat(rootDirectory + indexFile); err == nil {
				filePath = indexFile
				break
			}
		}
	}

	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}

	fullPath := rootDirectory + filePath

	// 檢查文件或目錄是否存在
	fileInfo, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		// 如果文件不存在，服務 404 頁面
		http.ServeFile(w, r, rootDirectory+notFoundPage)
		return
	}

	// 如果請求的是目錄，使用 http.ServeFile 來處理目錄
	if fileInfo.IsDir() {
		http.ServeFile(w, r, fullPath)
		return
	}

	// 如果請求的是文件，使用 http.ServeContent 來支持多线程下载
	file, err := os.Open(fullPath)
	if err != nil {
		// 如果無法打開文件，服務 403 頁面
		http.ServeFile(w, r, rootDirectory+forbiddenPage)
		return
	}
	defer file.Close()

	// 根據文件擴展名設置 Content-Type
	switch {
	case strings.HasSuffix(filePath, ".html"):
		w.Header().Set("Content-Type", "text/html")
	case strings.HasSuffix(filePath, ".css"):
		w.Header().Set("Content-Type", "text/css")
	case strings.HasSuffix(filePath, ".js"):
		w.Header().Set("Content-Type", "application/javascript")
	}

	// 使用 http.ServeContent 來傳輸文件內容，支持多线程下载
	http.ServeContent(w, r, fileInfo.Name(), fileInfo.ModTime(), file)

	// 在成功傳輸後更新 bytesSent
	bytesSent := fileInfo.Size()
	logRequestDetails(r, filePath, bytesSent)
}

func main() {
    loadConfig() // 加载配置

    // 设置处理函数
    http.HandleFunc("/", handleRequest)

    // 处理IPv6地址
    addr := ipAddress
    if strings.Contains(ipAddress, ":") {
        addr = fmt.Sprintf("[%s]", ipAddress) // 包裹IPv6地址在方括号中
    }
    serverAddr := fmt.Sprintf("%s:%s", addr, port)

    srv := &http.Server{
        Addr:           serverAddr,
        Handler:        nil, // 使用 http.DefaultServeMux
        ReadTimeout:    10 * time.Second,
        WriteTimeout:   10 * time.Second,
        IdleTimeout:    15 * time.Second,
        MaxHeaderBytes: 1 << 20,
    }

    fmt.Printf("服务器运行在 http://%s\n", serverAddr)
    pid := os.Getpid()
    fmt.Printf("PID: %d\n", pid)

    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("启动服务器时发生错误: %v", err)
        }
    }()

    // 捕捉 Ctrl+C 信号
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)

    // 等待 Ctrl+C 信号
    <-c

    // 打印绿色退出消息
    fmt.Print("\033[1;32m服务器退出...\033[0m\n")

    // 优雅关闭服务器
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := srv.Shutdown(ctx); err != nil {
        log.Fatalf("服务器关闭时发生错误: %v", err)
    }
}
