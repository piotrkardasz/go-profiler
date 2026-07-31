package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUIHandlerServesIndexHTML(t *testing.T) {
	assets := UIDistFS()
	if assets == nil {
		t.Skip("UI dist not built/embedded")
	}

	h := NewUIHandler(UIConfig{
		RoutePrefix: "/_profiler",
		Assets:      assets,
	})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux, "/_profiler")

	req := httptest.NewRequest(http.MethodGet, "/_profiler/", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<div id=\"app\">") {
		t.Error("response does not contain Vue app mount point")
	}
	if !strings.Contains(body, "Go Profiler") {
		t.Error("response does not contain 'Go Profiler' title")
	}
}

func TestUIHandlerServesSPARoutes(t *testing.T) {
	assets := UIDistFS()
	if assets == nil {
		t.Skip("UI dist not built/embedded")
	}

	h := NewUIHandler(UIConfig{
		RoutePrefix: "/_profiler",
		Assets:      assets,
	})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux, "/_profiler")

	// SPA route (no file extension) should return index.html
	req := httptest.NewRequest(http.MethodGet, "/_profiler/profile/abc123", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("SPA route status: got %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<div id=\"app\">") {
		t.Error("SPA route should return index.html")
	}
}

func TestUIHandlerServesStaticAssets(t *testing.T) {
	assets := UIDistFS()
	if assets == nil {
		t.Skip("UI dist not built/embedded")
	}

	h := NewUIHandler(UIConfig{
		RoutePrefix: "/_profiler",
		Assets:      assets,
	})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux, "/_profiler")

	// Static asset (favicon)
	req := httptest.NewRequest(http.MethodGet, "/_profiler/favicon.svg", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("static asset status: got %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<svg") {
		t.Error("favicon should be an SVG file")
	}
}

func TestUIHandlerNoAssets(t *testing.T) {
	h := NewUIHandler(UIConfig{
		RoutePrefix: "/_profiler",
		Assets:      nil,
	})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux, "/_profiler")

	req := httptest.NewRequest(http.MethodGet, "/_profiler/", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when no assets, got %d", rec.Code)
	}
}

func TestUIHandlerDevMode(t *testing.T) {
	// Create a mock "Vite dev server"
	mockVite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("vite-dev-response"))
	}))
	defer mockVite.Close()

	h := NewUIHandler(UIConfig{
		RoutePrefix:  "/_profiler",
		DevMode:      true,
		DevServerURL: mockVite.URL,
	})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux, "/_profiler")

	req := httptest.NewRequest(http.MethodGet, "/_profiler/", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("dev mode status: got %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if body != "vite-dev-response" {
		t.Errorf("dev mode body: got %q, want %q", body, "vite-dev-response")
	}
}

func TestUIDistFSNotNil(t *testing.T) {
	assets := UIDistFS()
	if assets == nil {
		t.Skip("UI dist not built/embedded")
	}
	// Verify we can read the index.html
	f, err := assets.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open index.html from embedded FS: %v", err)
	}
	f.Close()
}

func TestHasFileExtension(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/index.html", true},
		{"/assets/index-abc.js", true},
		{"/assets/style-def.css", true},
		{"/favicon.svg", true},
		{"/profile/abc123", false},
		{"/", false},
		{"", false},
		{"/some/path/without/extension", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := hasFileExtension(tt.path)
			if got != tt.expected {
				t.Errorf("hasFileExtension(%q): got %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}
