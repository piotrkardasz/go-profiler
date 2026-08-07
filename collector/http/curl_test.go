package http

import (
	"strings"
	"testing"
)

func TestBuildCurlCommand_GET(t *testing.T) {
	entry := &HTTPCallEntry{
		Method: "GET",
		URL:    "http://api.example.com/items?page=1",
	}

	cmd := buildCurlCommand(entry, defaultOptions())

	if !strings.HasPrefix(cmd, "curl ") {
		t.Errorf("expected to start with 'curl ', got %q", cmd)
	}
	// GET should not have -X GET
	if strings.Contains(cmd, "-X GET") {
		t.Error("GET requests should not include -X GET")
	}
	if !strings.Contains(cmd, "'http://api.example.com/items?page=1'") {
		t.Errorf("expected URL in curl, got %q", cmd)
	}
}

func TestBuildCurlCommand_POST_WithBody(t *testing.T) {
	entry := &HTTPCallEntry{
		Method:      "POST",
		URL:         "http://api.example.com/items",
		RequestBody: `{"name":"test","value":42}`,
		RequestHeaders: map[string][]string{
			"Content-Type": {"application/json"},
		},
	}

	cmd := buildCurlCommand(entry, defaultOptions())

	if !strings.Contains(cmd, "curl -X POST") {
		t.Errorf("expected 'curl -X POST', got %q", cmd)
	}
	if !strings.Contains(cmd, "-H 'Content-Type: application/json'") {
		t.Errorf("expected Content-Type header, got %q", cmd)
	}
	if !strings.Contains(cmd, `-d '{"name":"test","value":42}'`) {
		t.Errorf("expected body in curl, got %q", cmd)
	}
}

func TestBuildCurlCommand_PUT(t *testing.T) {
	entry := &HTTPCallEntry{
		Method: "PUT",
		URL:    "http://api.example.com/items/1",
	}

	cmd := buildCurlCommand(entry, defaultOptions())
	if !strings.Contains(cmd, "curl -X PUT") {
		t.Errorf("expected 'curl -X PUT', got %q", cmd)
	}
}

func TestBuildCurlCommand_RedactedHeaders(t *testing.T) {
	entry := &HTTPCallEntry{
		Method: "GET",
		URL:    "http://api.example.com/secret",
		RequestHeaders: map[string][]string{
			"Authorization": {"Bearer token123"},
			"Accept":        {"application/json"},
		},
	}

	opts := defaultOptions()
	cmd := buildCurlCommand(entry, opts)

	// Authorization should be excluded from curl (it's redacted)
	if strings.Contains(cmd, "token123") {
		t.Error("redacted header value should not appear in curl")
	}
	if strings.Contains(cmd, "Authorization") {
		t.Error("redacted header name should not appear in curl")
	}
	// Accept should be included
	if !strings.Contains(cmd, "-H 'Accept: application/json'") {
		t.Errorf("expected Accept header in curl, got %q", cmd)
	}
}

func TestBuildCurlCommand_TransportHeadersExcluded(t *testing.T) {
	entry := &HTTPCallEntry{
		Method: "GET",
		URL:    "http://api.example.com/",
		RequestHeaders: map[string][]string{
			"Content-Length":    {"42"},
			"Host":             {"api.example.com"},
			"Accept-Encoding":  {"gzip"},
			"Accept":           {"text/html"},
			"Connection":       {"keep-alive"},
			"Transfer-Encoding": {"chunked"},
		},
	}

	cmd := buildCurlCommand(entry, &options{redactHeaders: make(map[string]bool)})

	// Transport headers should be excluded
	if strings.Contains(cmd, "Content-Length") {
		t.Error("Content-Length should be excluded")
	}
	if strings.Contains(cmd, "'Host:") {
		t.Error("Host should be excluded")
	}
	if strings.Contains(cmd, "Accept-Encoding") {
		t.Error("Accept-Encoding should be excluded")
	}
	// Accept should be included
	if !strings.Contains(cmd, "Accept: text/html") {
		t.Errorf("expected Accept header, got %q", cmd)
	}
}

func TestBuildCurlCommand_SingleQuoteEscaping(t *testing.T) {
	entry := &HTTPCallEntry{
		Method:      "POST",
		URL:         "http://api.example.com/search",
		RequestBody: `{"query":"it's a test"}`,
	}

	cmd := buildCurlCommand(entry, &options{redactHeaders: make(map[string]bool)})

	// Single quotes in body should be escaped
	if !strings.Contains(cmd, `it'\''s a test`) {
		t.Errorf("expected escaped single quote in body, got %q", cmd)
	}
}

func TestBuildCurlCommand_NoHeaders(t *testing.T) {
	entry := &HTTPCallEntry{
		Method: "DELETE",
		URL:    "http://api.example.com/items/1",
	}

	cmd := buildCurlCommand(entry, defaultOptions())

	expected := "curl -X DELETE 'http://api.example.com/items/1'"
	if cmd != expected {
		t.Errorf("expected %q, got %q", expected, cmd)
	}
}

func TestEscapeSingleQuotes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"it's", "it'\\''s"},
		{"'start", "'\\''start"},
		{"end'", "end'\\''"},
		{"no quotes", "no quotes"},
	}

	for _, tt := range tests {
		result := escapeSingleQuotes(tt.input)
		if result != tt.expected {
			t.Errorf("escapeSingleQuotes(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
