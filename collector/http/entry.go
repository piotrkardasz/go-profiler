package http

import "time"

// HTTPCallEntry represents a single captured outbound HTTP call.
type HTTPCallEntry struct {
	Index           int                 `json:"index"`
	Service         string              `json:"service"`
	Method          string              `json:"method"`
	URL             string              `json:"url"`
	RequestHeaders  map[string][]string `json:"request_headers,omitempty"`
	RequestBody     string              `json:"request_body,omitempty"`
	RequestSize     int64               `json:"request_size"`
	StatusCode      int                 `json:"status_code"`
	ResponseHeaders map[string][]string `json:"response_headers,omitempty"`
	ResponseBody    string              `json:"response_body,omitempty"`
	ResponseSize    int64               `json:"response_size"`
	DurationMs      float64             `json:"duration_ms"`
	Error           string              `json:"error,omitempty"`
	Timestamp       time.Time           `json:"timestamp"`
	Backtrace       []string            `json:"backtrace,omitempty"`
	CurlCommand     string              `json:"curl_command,omitempty"`
}

// AnalysisResult holds analysis output for HTTP calls.
type AnalysisResult struct {
	SlowCalls      []HTTPCallEntry  `json:"slow_calls,omitempty"`
	FailedCalls    []HTTPCallEntry  `json:"failed_calls,omitempty"`
	DuplicateCalls []DuplicateGroup `json:"duplicate_calls,omitempty"`
}

// DuplicateGroup represents repeated identical calls.
type DuplicateGroup struct {
	Method  string `json:"method"`
	URL     string `json:"url"`
	Count   int    `json:"count"`
	Indices []int  `json:"indices"`
}

// Summary holds aggregate statistics for HTTP calls.
type Summary struct {
	TotalCalls      int            `json:"total_calls"`
	TotalDurationMs float64        `json:"total_duration_ms"`
	CallsPerService map[string]int `json:"calls_per_service"`
	FailedCount     int            `json:"failed_count"`
	SlowCount       int            `json:"slow_count"`
	DuplicateCount  int            `json:"duplicate_count"`
	SlowestCall     *HTTPCallEntry `json:"slowest_call,omitempty"`
}

// HTTPData is the top-level structure stored in Profile.CollectorData["http"].
type HTTPData struct {
	Calls    []HTTPCallEntry `json:"calls"`
	Analysis AnalysisResult  `json:"analysis"`
	Summary  Summary         `json:"summary"`
}
