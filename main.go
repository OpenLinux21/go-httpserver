package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
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
		line = strings.TrimSpace(line)

		// 跳过空行和注释行
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			log.Fatalf("配置行格式错误（第 %d 行）: %s", lineNumber+1, line)
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
			log.Printf("警告: 未知配置项（第 %d 行）: %s", lineNumber+1, line)
		}
	}
}

func generateRandomString(length int) string {
	// 生成指定长度的随机字符串
	rand.Seed(time.Now().UnixNano())
	randomBytes := make([]byte, length)
	for i := range randomBytes {
		randomBytes[i] = randomStringCharset[rand.Intn(len(randomStringCharset))]
	}
	return string(randomBytes)
}

func logRequestDetails(r *http.Request, filePath string, bytesSent int64) {
	// 按原格式记录日志至 latest.log 文件中
	clientIP := strings.Split(r.RemoteAddr, ":")[0]
	requestTime := time.Now().Format("2006-01-02 15:04:05")
	randomString := generateRandomString(16)
	logDetails := fmt.Sprintf("%s | ClientIP: %s | Port: %s | File: %s | Time: %s | BytesSent: %d\n",
		randomString, clientIP, port, filePath, requestTime, bytesSent)

	// 写入日志文件（格式保持不变）
	logFile, err := os.OpenFile("latest.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("打开日志文件错误: %v", err)
		return
	}
	defer logFile.Close()

	_, err = io.WriteString(logFile, logDetails)
	if err != nil {
		log.Printf("写入日志文件错误: %v", err)
	}
}

func autoIndex(w http.ResponseWriter, r *http.Request, directoryPath string) {
	entries, err := os.ReadDir(directoryPath)
	if err != nil {
		http.Error(w, "无法列出目录", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<html><head><title>Index of %s</title>", r.URL.Path)
	fmt.Fprintf(w, `<style>
		body { font-family: monospace; }
		table { width: 100%%; border-collapse: collapse; }
		th, td { text-align: left; padding: 5px; border-bottom: 1px solid #ddd; }
		a { text-decoration: none; }
		</style>`)
	fmt.Fprintf(w, "</head><body>")
	fmt.Fprintf(w, "<h1>Index of %s</h1>", r.URL.Path)
	fmt.Fprintf(w, "<table>")
	fmt.Fprintf(w, "<tr><th>Name</th><th>Last Modified</th><th>Size</th></tr>")

	// 若不在根目录，添加返回上级目录链接
	if r.URL.Path != "/" {
		parent := ".."
		fmt.Fprintf(w, `<tr>
			<td><a href="%s">%s</a></td>
			<td></td>
			<td>-</td>
			</tr>`, parent, "Parent Directory")
	}

	for _, entry := range entries {
		name := entry.Name()
		// 对文件或目录名称进行 URL 编码以处理特殊字符（如 '#'）
		encodedName := url.PathEscape(name)

		// 构造链接
		link := r.URL.Path
		if !strings.HasSuffix(link, "/") {
			link += "/"
		}
		link += encodedName

		// 目录在显示名称和链接后加斜杠
		displayName := name
		if entry.IsDir() {
			displayName += "/"
			link += "/"
		}

		// 获取文件信息以显示最后修改时间和大小
		info, err := entry.Info()
		modTime := ""
		size := ""
		if err == nil {
			modTime = info.ModTime().Format("2006-01-02 15:04")
			if info.IsDir() {
				size = "-"
			} else {
				size = fmt.Sprintf("%d", info.Size())
			}
		}
		fmt.Fprintf(w, `<tr>
			<td><a href="%s">%s</a></td>
			<td>%s</td>
			<td>%s</td>
			</tr>`, link, displayName, modTime, size)
	}
	fmt.Fprintf(w, "</table>")
	fmt.Fprintf(w, "<hr><address>Go HTTP Server</address>")
	fmt.Fprintf(w, "</body></html>")
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// 解码 URL 路径，处理特殊字符（例如 '#' 编码为 %23）
	filePath, err := url.PathUnescape(r.URL.Path)
	if err != nil {
		http.Error(w, "错误的请求", http.StatusBadRequest)
		return
	}
	fullPath := rootDirectory + filePath

	// 若请求根目录，则尝试查找配置中的索引文件
	if filePath == "/" {
		for _, indexFile := range indexFiles {
			indexFullPath := filepath.Join(rootDirectory, indexFile)
			if _, err := os.Stat(indexFullPath); err == nil {
				filePath = indexFile
				fullPath = filepath.Join(rootDirectory, filePath)
				break
			}
		}
	}

	// 确保路径以 "/" 开头
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
		fullPath = rootDirectory + filePath
	}

	// 检查文件或目录是否存在
	fileInfo, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		// 文件不存在时返回 404 页面
		http.ServeFile(w, r, filepath.Join(rootDirectory, notFoundPage))
		return
	}

	// 如果请求为目录
	if fileInfo.IsDir() {
		// 尝试在目录中查找索引文件
		foundIndex := false
		for _, indexFile := range indexFiles {
			indexFullPath := filepath.Join(fullPath, indexFile)
			if info, err := os.Stat(indexFullPath); err == nil && !info.IsDir() {
				http.ServeFile(w, r, indexFullPath)
				foundIndex = true
				break
			}
		}
		// 若未找到索引文件，则生成自动索引页面
		if !foundIndex {
			autoIndex(w, r, fullPath)
			return
		}
	}

	// 请求为文件时，尝试打开并服务其内容
	file, err := os.Open(fullPath)
	if err != nil {
		// 无法打开文件时返回 403 页面
		http.ServeFile(w, r, filepath.Join(rootDirectory, forbiddenPage))
		return
	}
	defer file.Close()

	// 根据文件扩展名设置 Content-Type
	switch {
	case strings.HasSuffix(filePath, ".html"):
		w.Header().Set("Content-Type", "text/html")
	case strings.HasSuffix(filePath, ".css"):
		w.Header().Set("Content-Type", "text/css")
	case strings.HasSuffix(filePath, ".js"):
		w.Header().Set("Content-Type", "application/javascript")
	}

	// 使用 http.ServeContent 支持多线程下载
	http.ServeContent(w, r, fileInfo.Name(), fileInfo.ModTime(), file)

	// 记录请求日志到文件（终端日志由 Gin 日志中间件输出）
	bytesSent := fileInfo.Size()
	logRequestDetails(r, filePath, bytesSent)
}

func main() {
	loadConfig() // 加载配置

	// 构造 Gin 引擎并添加中间件
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	// 所有请求交由 handleRequest 处理
	router.Any("/*filepath", func(c *gin.Context) {
		handleRequest(c.Writer, c.Request)
	})

	// 处理 IPv6 地址
	addr := ipAddress
	if strings.Contains(ipAddress, ":") {
		addr = fmt.Sprintf("[%s]", ipAddress)
	}
	serverAddr := fmt.Sprintf("%s:%s", addr, port)

	// 使用 http.Server 包装 Gin 引擎，便于优雅关闭
	srv := &http.Server{
		Addr:           serverAddr,
		Handler:        router,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    15 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	fmt.Printf("Server running at http://%s\n", serverAddr)
	pid := os.Getpid()
	fmt.Printf("PID: %d\n", pid)

	// 启动服务器
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("启动服务器错误: %v", err)
		}
	}()

	// 捕获 Ctrl+C 信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	// 打印绿色退出提示
	fmt.Print("\033[1;32mServer exiting...\033[0m\n")

	// 优雅关闭服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("关闭服务器错误: %v", err)
	}
}
