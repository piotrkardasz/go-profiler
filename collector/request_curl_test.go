package collector

import (
	"strings"
	"testing"
)

func TestBuildCurlCommandGET(t *testing.T) {
	input := &CurlInput{
		Method:  "GET",
		URL:     "http://localhost:8080/api/users",
		Headers: map[string][]string{"Accept": {"application/json"}},
	}

	result := BuildCurlCommand(input)

	// Should not include -X GET (it's curl's default)
	if strings.Contains(result, "-X GET") {
		t.Errorf("GET request should not include -X GET, got:\n%s", result)
	}
	if !strings.Contains(result, "curl 'http://localhost:8080/api/users'") {
		t.Errorf("expected URL in output, got:\n%s", result)
	}
	if !strings.Contains(result, "-H 'Accept: application/json'") {
		t.Errorf("expected Accept header, got:\n%s", result)
	}
}

func TestBuildCurlCommandPOSTWithBody(t *testing.T) {
	input := &CurlInput{
		Method: "POST",
		URL:    "http://localhost:8080/api/users",
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
			"Accept":       {"application/json"},
		},
		HasBody: true,
		Body:    `{"name":"John","email":"john@example.com"}`,
	}

	result := BuildCurlCommand(input)

	if !strings.Contains(result, "curl -X POST") {
		t.Errorf("expected -X POST, got:\n%s", result)
	}
	if !strings.Contains(result, "-H 'Content-Type: application/json'") {
		t.Errorf("expected Content-Type header, got:\n%s", result)
	}
	if !strings.Contains(result, `-d '{"name":"John","email":"john@example.com"}'`) {
		t.Errorf("expected body in -d flag, got:\n%s", result)
	}
}

func TestBuildCurlCommandPUTWithBody(t *testing.T) {
	input := &CurlInput{
		Method:  "PUT",
		URL:     "http://localhost:8080/api/users/1",
		Headers: map[string][]string{"Content-Type": {"application/json"}},
		HasBody: true,
		Body:    `{"name":"Jane"}`,
	}

	result := BuildCurlCommand(input)

	if !strings.Contains(result, "curl -X PUT") {
		t.Errorf("expected -X PUT, got:\n%s", result)
	}
	if !strings.Contains(result, `-d '{"name":"Jane"}'`) {
		t.Errorf("expected body, got:\n%s", result)
	}
}

func TestBuildCurlCommandNoBody(t *testing.T) {
	input := &CurlInput{
		Method:  "POST",
		URL:     "http://localhost:8080/api/trigger",
		Headers: map[string][]string{"X-Custom": {"value"}},
		HasBody: false,
	}

	result := BuildCurlCommand(input)

	if strings.Contains(result, "-d") {
		t.Errorf("should not include -d when no body, got:\n%s", result)
	}
}

func TestBuildCurlCommandBinaryBody(t *testing.T) {
	input := &CurlInput{
		Method:     "POST",
		URL:        "http://localhost:8080/upload",
		Headers:    map[string][]string{"Content-Type": {"multipart/form-data"}},
		IsBinary:   true,
		BinarySize: 2048576,
	}

	result := BuildCurlCommand(input)

	if !strings.Contains(result, "# Body: binary data (2048576 bytes) - not included") {
		t.Errorf("expected binary comment, got:\n%s", result)
	}
	if strings.Contains(result, "-d '") {
		t.Errorf("should not include -d flag for binary body, got:\n%s", result)
	}
}

func TestBuildCurlCommandRedactedHeaders(t *testing.T) {
	input := &CurlInput{
		Method: "GET",
		URL:    "http://localhost:8080/api/me",
		Headers: map[string][]string{
			"Authorization": {"[REDACTED]"},
			"Accept":        {"application/json"},
		},
	}

	result := BuildCurlCommand(input)

	if !strings.Contains(result, "-H 'Authorization: [REDACTED]'") {
		t.Errorf("expected redacted auth header, got:\n%s", result)
	}
}

func TestBuildCurlCommandTransportHeadersExcluded(t *testing.T) {
	input := &CurlInput{
		Method: "GET",
		URL:    "http://localhost:8080/",
		Headers: map[string][]string{
			"Content-Length":    {"42"},
			"Accept-Encoding":  {"gzip"},
			"Connection":       {"keep-alive"},
			"Host":             {"localhost:8080"},
			"Transfer-Encoding": {"chunked"},
			"X-Custom":         {"keep-me"},
		},
	}

	result := BuildCurlCommand(input)

	if strings.Contains(result, "Content-Length") {
		t.Errorf("should exclude Content-Length, got:\n%s", result)
	}
	if strings.Contains(result, "Accept-Encoding") {
		t.Errorf("should exclude Accept-Encoding, got:\n%s", result)
	}
	if strings.Contains(result, "'Connection:") {
		t.Errorf("should exclude Connection, got:\n%s", result)
	}
	if strings.Contains(result, "'Host:") {
		t.Errorf("should exclude Host, got:\n%s", result)
	}
	if strings.Contains(result, "Transfer-Encoding") {
		t.Errorf("should exclude Transfer-Encoding, got:\n%s", result)
	}
	if !strings.Contains(result, "-H 'X-Custom: keep-me'") {
		t.Errorf("should keep X-Custom header, got:\n%s", result)
	}
}

func TestBuildCurlCommandSingleQuoteEscaping(t *testing.T) {
	input := &CurlInput{
		Method:  "POST",
		URL:     "http://localhost:8080/api/data",
		Headers: map[string][]string{"Content-Type": {"application/json"}},
		HasBody: true,
		Body:    `{"message":"it's a test"}`,
	}

	result := BuildCurlCommand(input)

	if !strings.Contains(result, `it'\''s a test`) {
		t.Errorf("expected escaped single quote in body, got:\n%s", result)
	}
}

func TestBuildCurlCommandURLWithQueryParams(t *testing.T) {
	input := &CurlInput{
		Method:  "GET",
		URL:     "http://localhost:8080/api/users?page=2&limit=10",
		Headers: map[string][]string{},
	}

	result := BuildCurlCommand(input)

	if !strings.Contains(result, "'http://localhost:8080/api/users?page=2&limit=10'") {
		t.Errorf("expected URL with query params, got:\n%s", result)
	}
}

func TestBuildCurlCommandMultipleHeaderValues(t *testing.T) {
	input := &CurlInput{
		Method: "GET",
		URL:    "http://localhost:8080/",
		Headers: map[string][]string{
			"X-Multi": {"value1", "value2"},
		},
	}

	result := BuildCurlCommand(input)

	if !strings.Contains(result, "-H 'X-Multi: value1'") {
		t.Errorf("expected first header value, got:\n%s", result)
	}
	if !strings.Contains(result, "-H 'X-Multi: value2'") {
		t.Errorf("expected second header value, got:\n%s", result)
	}
}

func TestBuildCurlCommandHeadersSorted(t *testing.T) {
	input := &CurlInput{
		Method: "GET",
		URL:    "http://localhost:8080/",
		Headers: map[string][]string{
			"Z-Header": {"z"},
			"A-Header": {"a"},
			"M-Header": {"m"},
		},
	}

	result := BuildCurlCommand(input)

	aIdx := strings.Index(result, "A-Header")
	mIdx := strings.Index(result, "M-Header")
	zIdx := strings.Index(result, "Z-Header")

	if aIdx > mIdx || mIdx > zIdx {
		t.Errorf("headers not sorted alphabetically, got:\n%s", result)
	}
}

func TestBuildCurlCommandEmptyURL(t *testing.T) {
	input := &CurlInput{
		Method: "GET",
		URL:    "",
	}

	result := BuildCurlCommand(input)
	if result != "" {
		t.Errorf("expected empty result for empty URL, got:\n%s", result)
	}
}

func TestBuildCurlCommandNilInput(t *testing.T) {
	result := BuildCurlCommand(nil)
	if result != "" {
		t.Errorf("expected empty result for nil input, got:\n%s", result)
	}
}

func TestEscapeSingleQuotes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"it's", "it'\\''s"},
		{"'quoted'", "'\\''quoted'\\''"},
		{"no quotes here", "no quotes here"},
		{"", ""},
		{"a'b'c", "a'\\''b'\\''c"},
	}

	for _, tt := range tests {
		result := escapeSingleQuotes(tt.input)
		if result != tt.expected {
			t.Errorf("escapeSingleQuotes(%q): got %q, want %q", tt.input, result, tt.expected)
		}
	}
}
