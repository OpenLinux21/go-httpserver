package handlers

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenLinux21/go-httpserver/internal/config"
	"github.com/OpenLinux21/go-httpserver/internal/logger"
	"github.com/OpenLinux21/go-httpserver/internal/middleware"
)

func HandleError(w http.ResponseWriter, err error, statusCode int, logMsg string) {
	log.Printf("%s: %v", logMsg, err)
	http.Error(w, http.StatusText(statusCode), statusCode)
}

func AutoIndex(w http.ResponseWriter, r *http.Request, directoryPath string) {
	entries, err := os.ReadDir(directoryPath)
	if err != nil {
		HandleError(w, err, http.StatusInternalServerError, "Unable to list directory")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!DOCTYPE HTML PUBLIC \"-//W3C//DTD HTML 3.2 Final//EN\">\n")
	fmt.Fprintf(w, "<html>\n<head>\n<title>Index of %s</title>\n", r.URL.Path)
	fmt.Fprintf(w, `<style>
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
	fmt.Fprintf(w, "</head>\n<body>\n")
	fmt.Fprintf(w, "<h1>Index of %s</h1>\n", r.URL.Path)
	fmt.Fprintf(w, "<table>\n")
	fmt.Fprintf(w, "<tr><th class=\"name-cell\">Name</th><th class=\"date-cell\">Last modified</th><th class=\"size-cell\">Size</th></tr>\n")
	fmt.Fprintf(w, "<tr><th colspan=\"3\"><hr></th></tr>\n")

	if r.URL.Path != "/" {
		parent := ".."
		fmt.Fprintf(w, `<tr>
            <td class="name-cell"><a href="%s" class="parent-dir">⬆️ Parent Directory</a></td>
            <td class="date-cell">-</td>
            <td class="size-cell">-</td>
            </tr>`, parent)
	}

	// Process directories first
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		encodedName := url.PathEscape(name)

		link := r.URL.Path
		if !strings.HasSuffix(link, "/") {
			link += "/"
		}
		link += encodedName + "/"

		info, err := entry.Info()
		modTime := "-"
		if err == nil {
			modTime = info.ModTime().Format("2006-01-02 15:04")
		}

		fmt.Fprintf(w, `<tr>
            <td class="name-cell"><a href="%s" class="dir-name">📁 %s/</a></td>
            <td class="date-cell">%s</td>
            <td class="size-cell">-</td>
            </tr>`, link, name, modTime)
	}

	// Process files
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		encodedName := url.PathEscape(name)

		link := r.URL.Path
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

		fmt.Fprintf(w, `<tr>
            <td class="name-cell"><a href="%s">📄 %s</a></td>
            <td class="date-cell">%s</td>
            <td class="size-cell">%s</td>
            </tr>`, link, name, modTime, size)
	}

	fmt.Fprintf(w, "</table>\n")
	fmt.Fprintf(w, "<hr>\n")
	fmt.Fprintf(w, "<address>Go HTTP Server</address>\n")
	fmt.Fprintf(w, "</body>\n</html>")
}

func HandleRequest(w http.ResponseWriter, r *http.Request) {
	middleware.AddSecurityHeaders(w)

	// Clean and normalize the request path
	path := filepath.Clean(r.URL.Path)
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
				http.ServeFile(w, r, notFoundPath)
				return
			}
			HandleError(w, err, http.StatusNotFound, "File not found")
			return
		}
		HandleError(w, err, http.StatusInternalServerError, "Error accessing file")
		return
	}

	// Handle directory
	if fileInfo.IsDir() {
		// Check for index files
		for _, indexFile := range config.GlobalConfig.IndexFiles {
			indexPath := filepath.Join(fullPath, indexFile)
			if _, err := os.Stat(indexPath); err == nil {
				http.ServeFile(w, r, indexPath)
				return
			}
		}
		// If no index file found, show directory listing
		AutoIndex(w, r, fullPath)
		return
	}

	// Serve the file
	file, err := os.Open(fullPath)
	if err != nil {
		HandleError(w, err, http.StatusInternalServerError, "Error opening file")
		return
	}
	defer file.Close()

	// Get file size for logging
	fileInfo, err = file.Stat()
	if err != nil {
		HandleError(w, err, http.StatusInternalServerError, "Error getting file info")
		return
	}

	// Log the request
	logger.LogRequestDetails(r, fullPath, fileInfo.Size(), config.GlobalConfig.Port)

	// Serve the file
	http.ServeContent(w, r, fileInfo.Name(), fileInfo.ModTime(), file)
}
