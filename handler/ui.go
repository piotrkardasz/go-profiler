package handler

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// readSeeker combines io.Reader and io.Seeker for http.ServeContent.
type readSeeker interface {
	io.Reader
	io.Seeker
}

// UIHandler serves the profiler web UI, either from embedded assets
// or by proxying to a Vite dev server.
type UIHandler struct {
	prefix   string
	devMode  bool
	devProxy http.Handler
	assets   fs.FS
}

// UIConfig holds configuration for the UI handler.
type UIConfig struct {
	// RoutePrefix is the URL prefix for the profiler (e.g., "/_profiler").
	RoutePrefix string

	// DevMode enables proxying to the Vite dev server instead of serving
	// embedded assets. Controlled by GO_PROFILER_UI_DEV env var.
	DevMode bool

	// DevServerURL is the URL of the Vite dev server (default: "http://localhost:5173").
	DevServerURL string

	// Assets is the filesystem containing the built Vue app.
	// When nil and DevMode is false, the handler returns 404.
	Assets fs.FS
}

// NewUIHandler creates a UI handler based on the configuration.
// In dev mode, it reverse-proxies to the Vite dev server.
// In production mode, it serves embedded static files.
func NewUIHandler(cfg UIConfig) *UIHandler {
	h := &UIHandler{
		prefix:  strings.TrimRight(cfg.RoutePrefix, "/"),
		devMode: cfg.DevMode,
		assets:  cfg.Assets,
	}

	if cfg.DevMode {
		devURL := cfg.DevServerURL
		if devURL == "" {
			devURL = "http://localhost:5173"
		}
		target, _ := url.Parse(devURL)
		h.devProxy = httputil.NewSingleHostReverseProxy(target)
	}

	return h
}

// ServeHTTP handles requests to the profiler UI.
func (h *UIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.devMode && h.devProxy != nil {
		h.devProxy.ServeHTTP(w, r)
		return
	}

	if h.assets == nil {
		http.Error(w, "Profiler UI not available", http.StatusNotFound)
		return
	}

	// Strip the prefix to get the file path
	path := strings.TrimPrefix(r.URL.Path, h.prefix)
	path = strings.TrimPrefix(path, "/")

	// Default to index.html for root or SPA routes
	if path == "" || !hasFileExtension("/"+path) {
		path = "index.html"
	}

	// Open the file from embedded assets
	f, err := h.assets.Open(path)
	if err != nil {
		// File not found — serve index.html for SPA routing
		f, err = h.assets.Open("index.html")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
	}
	defer f.Close()

	// Get file info for content type and size
	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// If it's a directory, serve index.html
	if stat.IsDir() {
		f.Close()
		f, err = h.assets.Open("index.html")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		defer f.Close()
		stat, _ = f.Stat()
	}

	// Set content type based on extension
	contentType := inferContentType(path)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	// Serve the file using http.ServeContent for proper caching headers
	if seeker, ok := f.(readSeeker); ok {
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), seeker)
	} else {
		// Fallback: read all and write
		data, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		w.Write(data)
	}
}

// RegisterRoutes registers the UI handler on the given mux.
// It handles all paths under the prefix that aren't API routes.
func (h *UIHandler) RegisterRoutes(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	// Catch-all for UI routes (must come after API routes in specificity)
	mux.Handle(prefix+"/", h)
}

// hasFileExtension checks if a path has a file extension (static asset).
func hasFileExtension(path string) bool {
	// Check the last path segment for a dot
	lastSlash := strings.LastIndex(path, "/")
	lastSegment := path
	if lastSlash >= 0 {
		lastSegment = path[lastSlash:]
	}
	return strings.Contains(lastSegment, ".")
}

// inferContentType returns the MIME type for common static file extensions.
func inferContentType(path string) string {
	switch {
	case strings.HasSuffix(path, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(path, ".js"):
		return "application/javascript"
	case strings.HasSuffix(path, ".css"):
		return "text/css"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".json"):
		return "application/json"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(path, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(path, ".woff"):
		return "font/woff"
	default:
		return ""
	}
}
