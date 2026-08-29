package handlers

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/OpenLinux21/go-httpserver/internal/config"
	"github.com/gin-gonic/gin"
)

const directoryTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Index of {{.Path}}</title>
<style>
body { max-width: 72rem; margin: 2rem auto; padding: 0 1rem; font-family: system-ui, sans-serif; color: #222; }
h1 { font-size: 1.5rem; }
table { width: 100%; border-collapse: collapse; font-family: ui-monospace, monospace; }
th, td { padding: .45rem .7rem; text-align: left; border-bottom: 1px solid #ddd; }
th { background: #f4f4f4; }
a { color: #0645ad; text-decoration: none; }
a:hover { text-decoration: underline; }
.size { text-align: right; }
</style>
</head>
<body>
<h1>Index of {{.Path}}</h1>
<table>
<thead><tr><th>Name</th><th>Last modified</th><th class="size">Size</th></tr></thead>
<tbody>
{{if .Parent}}<tr><td><a href="../">../</a></td><td>-</td><td class="size">-</td></tr>{{end}}
{{range .Entries}}<tr><td><a href="{{.URL}}">{{.Name}}</a></td><td>{{.Modified}}</td><td class="size">{{.Size}}</td></tr>{{end}}
</tbody>
</table>
<p>Go HTTP Server</p>
</body>
</html>`

var autoIndexTemplate = template.Must(template.New("directory").Parse(directoryTemplate))

const maxErrorPageSize = 1 << 20

type Handler struct {
	cfg  config.Config
	root *os.Root
}

type directoryEntry struct {
	Name     string
	URL      string
	Modified string
	Size     string
	IsDir    bool
}

type directoryData struct {
	Path    string
	Parent  bool
	Entries []directoryEntry
}

func New(cfg config.Config) (*Handler, error) {
	root, err := os.OpenRoot(cfg.RootDirectory)
	if err != nil {
		return nil, fmt.Errorf("open website root: %w", err)
	}
	return &Handler{cfg: cfg, root: root}, nil
}

func (h *Handler) Close() error {
	return h.root.Close()
}

func (h *Handler) Serve(c *gin.Context) {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		c.Header("Allow", "GET, HEAD")
		c.AbortWithStatus(http.StatusMethodNotAllowed)
		return
	}

	requestPath := pathpkg.Clean("/" + c.Request.URL.Path)
	name := strings.TrimPrefix(requestPath, "/")
	if name == "" {
		name = "."
	}

	info, err := h.root.Stat(name)
	if err != nil {
		h.handleFileError(c, err)
		return
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		h.serveError(c, http.StatusForbidden, fmt.Errorf("refusing special file %q", name))
		return
	}
	file, err := h.root.Open(name)
	if err != nil {
		h.handleFileError(c, err)
		return
	}
	defer file.Close()
	if info.IsDir() {
		h.serveDirectory(c, requestPath, name, file)
		return
	}
	h.serveContent(c, file, info)
}

func (h *Handler) serveDirectory(c *gin.Context, requestPath, name string, directory *os.File) {
	if !strings.HasSuffix(c.Request.URL.Path, "/") {
		location := &url.URL{Path: c.Request.URL.Path + "/", RawQuery: c.Request.URL.RawQuery}
		http.Redirect(c.Writer, c.Request, location.String(), http.StatusMovedPermanently)
		return
	}

	for _, indexName := range h.cfg.IndexFiles {
		indexPath := pathpkg.Join(name, filepathToSlash(indexName))
		info, err := h.root.Stat(indexPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			h.handleFileError(c, err)
			return
		}
		if !info.Mode().IsRegular() {
			continue
		}
		file, err := h.root.Open(indexPath)
		if err == nil {
			h.serveContent(c, file, info)
			file.Close()
			return
		}
		h.handleFileError(c, err)
		return
	}

	entries, err := directory.ReadDir(-1)
	if err != nil {
		h.handleFileError(c, err)
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	data := directoryData{Path: requestPath, Parent: requestPath != "/"}
	for _, entry := range entries {
		item := directoryEntry{Name: entry.Name(), URL: url.PathEscape(entry.Name()), Size: "-", IsDir: entry.IsDir()}
		if entry.IsDir() {
			item.Name += "/"
			item.URL += "/"
		}
		if info, err := entry.Info(); err == nil {
			item.Modified = info.ModTime().Format("2006-01-02 15:04")
			if info.Mode().IsRegular() {
				item.Size = fmt.Sprintf("%d", info.Size())
			}
		} else {
			item.Modified = "-"
		}
		data.Entries = append(data.Entries, item)
	}

	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if c.Request.Method == http.MethodHead {
		return
	}
	if err := autoIndexTemplate.Execute(c.Writer, data); err != nil {
		log.Printf("render directory listing: %v", err)
	}
}

func (h *Handler) serveContent(c *gin.Context, file *os.File, info fs.FileInfo) {
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
}

func (h *Handler) handleFileError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		h.serveError(c, http.StatusNotFound, err)
	case errors.Is(err, fs.ErrPermission):
		h.serveError(c, http.StatusForbidden, err)
	default:
		h.serveError(c, http.StatusInternalServerError, err)
	}
}

func (h *Handler) serveError(c *gin.Context, status int, cause error) {
	if status >= 500 {
		log.Printf("request failed: %v", cause)
	}

	page := ""
	if status == http.StatusNotFound {
		page = h.cfg.NotFoundPage
	} else if status == http.StatusForbidden {
		page = h.cfg.ForbiddenPage
	}
	if page != "" {
		page = filepathToSlash(page)
		info, statErr := h.root.Stat(page)
		if statErr == nil && info.Mode().IsRegular() && info.Size() <= maxErrorPageSize {
			file, openErr := h.root.Open(page)
			if openErr != nil {
				c.String(status, http.StatusText(status))
				return
			}
			data, readErr := io.ReadAll(io.LimitReader(file, maxErrorPageSize+1))
			file.Close()
			if readErr != nil || len(data) > maxErrorPageSize {
				c.String(status, http.StatusText(status))
				return
			}
			contentType := mime.TypeByExtension(pathpkg.Ext(page))
			if contentType == "" {
				contentType = http.DetectContentType(data)
			}
			c.Header("Content-Type", contentType)
			c.Status(status)
			if c.Request.Method != http.MethodHead {
				_, _ = c.Writer.Write(data)
			}
			return
		}
	}
	c.String(status, http.StatusText(status))
}

func filepathToSlash(name string) string {
	return strings.ReplaceAll(name, "\\", "/")
}
