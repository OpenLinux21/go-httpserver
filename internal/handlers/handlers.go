package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenLinux21/go-httpserver/internal/config"
	"github.com/OpenLinux21/go-httpserver/internal/logger"
	"github.com/OpenLinux21/go-httpserver/internal/middleware"
	"github.com/gin-gonic/gin"
)

func HandleError(ctx *gin.Context, err error, statusCode int, logMsg string) {
	ctx.Error(err)
	ctx.String(statusCode, http.StatusText(statusCode))
}

func AutoIndex(ctx *gin.Context, rpath string, directoryPath string) {
	entries, err := os.ReadDir(directoryPath)
	if err != nil {
		HandleError(ctx, err, 500, "Unable to list directory")
		return
	}

	ctx.Header("Content-Type", "text/html; charset=utf-8")
	ctx.Writer.WriteString("<!DOCTYPE HTML PUBLIC \"-//W3C//DTD HTML 3.2 Final//EN\">\n")
	ctx.Writer.WriteString(fmt.Sprintf("<html>\n<head>\n<title>Index of %s</title>\n", rpath))
	ctx.Writer.WriteString(`<style>
		body { font-family: Arial, sans-serif; }
		h1 { font-size: 1.5em; margin: 0.5em 0; }
		table { width: 100%%; border-collapse: collapse; font-family: monospace; }
		th { text-align: left; padding: 0.5em 1em; background: #e4e4e4; border-bottom: 1px solid #ccc; }
		td { padding: 0.25em 1em; }
		tr:hover td { background: #f4f4f4; }
		a { text-decoration: none; color: #00e; }
		a:hover { text-decoration: underline; color: #00f; }
		hr { border: 0; border-top: 1px solid #ccc; margin: 1em 0; }
		.name-cell { min-width: 35%%; }
		.date-cell { min-width: 20%%; }
		.size-cell { min-width: 10%%; }
		.icon { width: 20px; height: 20px; vertical-align: middle; margin-right: 5px; }
		.parent-dir { color: #666; }
		.dir-name { font-weight: bold; }
		address { font-size: 0.8em; font-style: italic; color: #666; margin-top: 1em; }
	</style>`)
	ctx.Writer.WriteString("</head>\n<body>\n")
	ctx.Writer.WriteString(fmt.Sprintf("<h1>Index of %s</h1>\n", rpath))
	ctx.Writer.WriteString("<table>\n")
	ctx.Writer.WriteString("<tr><th class=\"name-cell\">Name</th><th class=\"date-cell\">Last modified</th><th class=\"size-cell\">Size</th></tr>\n")
	ctx.Writer.WriteString("<tr><th colspan=\"3\"><hr></th></tr>\n")

	if rpath != "/" {
		parent := ".."
		ctx.Writer.WriteString(fmt.Sprintf(`<tr>
			<td class="name-cell"><a href="%s" class="parent-dir">↑ Parent Directory</a></td>
			<td class="date-cell">-</td>
			<td class="size-cell">-</td>
			</tr>`, parent))
	}

	// Process directories first
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		encodedName := url.PathEscape(name)

		link := rpath
		if !strings.HasSuffix(link, "/") {
			link += "/"
		}
		link += encodedName + "/"

		info, err := entry.Info()
		modTime := "-"
		if err == nil {
			modTime = info.ModTime().Format("2006-01-02 15:04")
		}

		ctx.Writer.WriteString(fmt.Sprintf(`<tr>
			<td class="name-cell"><a href="%s" class="dir-name">📁 %s/</a></td>
			<td class="date-cell">%s</td>
			<td class="size-cell">-</td>
			</tr>`, link, name, modTime))
	}

	// Process files
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		encodedName := url.PathEscape(name)

		link := rpath
		if !strings.HasSuffix(link, "/") {
			link += "/"
		}
		link += encodedName

		info, err := entry.Info()
		modTime, size := "-", "-"
		if err == nil {
			modTime = info.ModTime().Format("2006-01-02 15:04")
			size = fmt.Sprintf("%d", info.Size())
		}

		ctx.Writer.WriteString(fmt.Sprintf(`<tr>
			<td class="name-cell"><a href="%s">📄 %s</a></td>
			<td class="date-cell">%s</td>
			<td class="size-cell">%s</td>
			</tr>`, link, name, modTime, size))
	}

	ctx.Writer.WriteString("</table>\n")
	ctx.Writer.WriteString("<hr>\n")
	ctx.Writer.WriteString("<address>Go HTTP Server</address>\n")
	ctx.Writer.WriteString("</body>\n</html>")
}

// HandleGinRequest is the gin-compatible entry point for serving static files and directory listings
func HandleGinRequest(ctx *gin.Context) {
	middleware.AddSecurityHeaders(ctx.Writer)

	// Clean and normalize the request path
	path := filepath.Clean(ctx.Request.URL.Path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Construct the full file path
	fullPath := filepath.Join(config.GlobalConfig.RootDirectory, path)

	// Check if the path exists
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Try to serve 404 page
			notFoundPath := filepath.Join(config.GlobalConfig.RootDirectory, config.GlobalConfig.NotFoundPage)
			if _, err := os.Stat(notFoundPath); err == nil {
				ctx.File(notFoundPath)
				return
			}
			HandleError(ctx, err, 404, "File not found")
			return
		}
		HandleError(ctx, err, 500, "Error accessing file")
		return
	}

	// Handle directory
	if fileInfo.IsDir() {
		// Check for index files
		for _, indexFile := range config.GlobalConfig.IndexFiles {
			indexPath := filepath.Join(fullPath, indexFile)
			if _, err := os.Stat(indexPath); err == nil {
				ctx.File(indexPath)
				return
			}
		}
		// If no index file found, show directory listing
		AutoIndex(ctx, path, fullPath)
		return
	}

	// Serve the file
	// Log the request: use gin client IP extraction
	clientIP := ctx.ClientIP()

	// Get file size for logging
	fi, err := os.Stat(fullPath)
	if err == nil {
		logger.LogRequestDetails(clientIP, fullPath, fi.Size(), config.GlobalConfig.Port)
	}

	ctx.File(fullPath)
}
