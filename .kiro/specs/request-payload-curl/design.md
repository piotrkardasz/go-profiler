# Design: Request Payload Capture & Curl Command Generation

## Technical Design Document

### 1. System Architecture

The feature extends the existing `RequestCollector` in the root module to optionally capture HTTP request bodies and generate ready-to-use `curl` commands. Body capture is gated by an environment variable (disabled by default), while curl generation always runs when the request collector is active.

```
┌──────────────────────────────────────────────────────────────────┐
│  HTTP Request (with body)                                         │
├──────────────────────────────────────────────────────────────────┤
│  Profiler Middleware                                              │
│  ├─► Body Capture (if enabled):                                  │
│  │   ├─► Read r.Body into buffer (up to max size)                │
│  │   ├─► Replace r.Body with io.NopCloser(bytes.NewReader(buf))  │
│  │   └─► Store buffer in context for later collection            │
│  └─► Application Handler (consumes r.Body normally)              │
├──────────────────────────────────────────────────────────────────┤
│  After Response: Profiler calls RequestCollector.Collect()         │
│  ├─► Capture method, URL, headers, query params (existing)       │
│  ├─► Retrieve body from context (if captured)                    │
│  │   ├─► Detect binary content → placeholder                    │
│  │   └─► Apply truncation indicator if oversized                 │
│  ├─► Apply header redaction (respects PROFILER_REDACT_HEADERS)   │
│  ├─► Generate curl command via buildCurlCommand()                │
│  │   ├─► Method + URL                                            │
│  │   ├─► Filtered headers (skip transport-level)                 │
│  │   └─► Body (if captured and not binary)                       │
│  └─► Return RequestData (JSON-serializable)                      │
└──────────────────────────────────────────────────────────────────┘
```

### 2. File Structure

```
collector/
├── collector.go              # Existing: Collector interface
├── request.go                # Modified: RequestCollector with options, body capture, curl gen
├── request_options.go        # New: Functional options for RequestCollector
├── request_body.go           # New: Body reading, content type detection, context helpers
├── request_curl.go           # New: Curl command builder (pure function)
├── request_test.go           # Modified: Extended tests
├── request_body_test.go      # New: Body capture tests
├── request_curl_test.go      # New: Curl generation tests
├── memory.go                 # Existing
├── timing.go                 # Existing
└── config.go                 # Existing

middleware.go                 # Modified: Body capture hook before handler

ui/src/components/panels/
├── RequestPanel.vue          # Modified: Add body display + curl command section
├── GormPanel.vue             # Existing
├── ...
```

### 3. Core Design Decisions

#### 3.1 Body Capture in Middleware (Before Handler)

**Decision:** Read and buffer the request body in the profiler middleware *before* invoking the application handler, then replace `r.Body` with a new reader over the buffered bytes.

**Rationale:**
- The `Collect()` method runs *after* the handler completes. By then, `r.Body` has been consumed and is no longer readable.
- Reading upfront and replacing with `io.NopCloser(bytes.NewReader(buf))` is a well-established Go pattern (used by middleware libraries like chi, echo).
- The buffer is stored in the request context so `Collect()` can retrieve it without additional I/O.
- When body capture is disabled, the middleware skips this entirely — zero overhead on the hot path.

**Implementation:**
```go
// In middleware.go, before next.ServeHTTP(wrapped, r):
if p.requestCollector != nil && p.requestCollector.bodyCaptureEnabled {
    ctx, r = p.requestCollector.CaptureBody(ctx, r)
}
```

#### 3.2 Context-Based Body Storage

**Decision:** Store the captured body bytes in the request context using a typed key.

**Rationale:**
- Matches the existing pattern (timing uses `WithStartTime(ctx)`, memory uses `WithMemoryStats(ctx)`).
- No shared mutable state between middleware and collector — the buffer is immutable once written.
- Context is already passed to `Collect()`.
- If body capture is disabled, no context value is set (zero allocation).

**Implementation:**
```go
type bodyContextKey struct{}

type capturedBody struct {
    content   string // The captured body (possibly truncated)
    size      int64  // Original full body size
    truncated bool   // Whether content was truncated
    binary    bool   // Whether binary content was detected
}
```

#### 3.3 RequestCollector Gains State (Options)

**Decision:** Transform `RequestCollector` from a zero-field struct into one holding configuration options, while keeping it stateless per-request (no mutable request state).

**Rationale:**
- Currently `RequestCollector` is `struct{}` — no configuration at all.
- Adding options (body capture toggle, max size, content types, header redaction) requires storing config.
- Config is immutable after construction — set once via `NewRequestCollector(opts...)`.
- Per-request state still lives in context, not in the struct.
- Maintains the same `Collector` interface contract.

**Constructor change:**
```go
// Before:
func NewRequestCollector() *RequestCollector

// After:
func NewRequestCollector(opts ...RequestOption) *RequestCollector
```

Existing code calling `NewRequestCollector()` with no args continues to work unchanged (all options have safe defaults).

#### 3.4 Header Redaction Toggle

**Decision:** Make the `[REDACTED]` behavior for sensitive headers switchable via `PROFILER_REDACT_HEADERS` env var (default: `true`).

**Rationale:**
- In local development, developers often want fully working curl commands with real auth tokens.
- The profiler is a dev tool — opt-out redaction for local environments makes it more useful.
- Default stays secure (redacted) so no accidental exposure in shared environments.
- Env var is consistent with how other profiler settings work (`PROFILER_CAPTURE_BODY`, `PROFILER_MASK_SECRETS`).

**Logic change in `sanitizeHeaders`:**
```go
func (c *RequestCollector) sanitizeHeaders(h http.Header) map[string][]string {
    // ...
    if c.redactHeaders && sensitiveHeaders[k] {
        result[k] = []string{"[REDACTED]"}
    } else {
        result[k] = v
    }
}
```

#### 3.5 Curl Command as Pure Function

**Decision:** Implement curl generation as a standalone, exported function `BuildCurlCommand(data *CurlInput) string` in a separate file.

**Rationale:**
- Testable in isolation with table-driven tests.
- Can be reused by future features (HAR export, Postman collection generation).
- Separation of concerns: `Collect()` gathers data, `BuildCurlCommand()` formats output.
- No side effects, no I/O — pure string transformation.

**Input struct:**
```go
type CurlInput struct {
    Method      string
    URL         string
    Headers     map[string][]string // Already sanitized (redacted or not)
    Body        string              // Empty if not captured or binary
    HasBody     bool                // True if -d should be included
    IsBinary    bool                // True if body is binary (comment instead of -d)
    BinarySize  int64               // Original size for the comment
}
```

#### 3.6 Transport Header Exclusion

**Decision:** Exclude transport-level headers from the curl command to keep it focused and readable.

**Excluded headers:**
```go
var curlExcludedHeaders = map[string]bool{
    "Content-Length":    true,
    "Accept-Encoding":  true,
    "Connection":       true,
    "Host":             true, // curl derives from URL
    "Transfer-Encoding": true,
    "Upgrade":          true,
}
```

**Rationale:**
- `Content-Length` is auto-calculated by curl from `-d` body.
- `Accept-Encoding` causes curl to request compressed response (confusing for debugging).
- `Host` is derived from the URL by curl.
- `Connection` and `Transfer-Encoding` are transport-level, not meaningful when replaying.
- Application-level headers (Content-Type, Authorization, custom X- headers) are preserved.

#### 3.7 Binary Content Detection

**Decision:** Use a content-type whitelist to determine what's "text" (captured as-is) vs "binary" (placeholder).

**Text content types (captured as-is):**
```go
var textContentTypes = []string{
    "application/json",
    "application/xml",
    "application/x-www-form-urlencoded",
    "application/graphql",
    "application/javascript",
    "application/yaml",
    "text/",  // prefix match: text/plain, text/html, text/csv, etc.
}
```

**Rationale:**
- Checking actual bytes for binary detection (like the `\x00` heuristic) requires reading and scanning.
- Content-Type is already available as a header — no extra I/O needed.
- Whitelist approach is safer (default to "binary" for unknown types).
- The `text/` prefix match covers all text subtypes.

### 4. Data Structures

#### 4.1 Extended RequestData

```go
type RequestData struct {
    // Existing fields (unchanged)
    Method      string              `json:"method"`
    URL         string              `json:"url"`
    Host        string              `json:"host"`
    RemoteAddr  string              `json:"remote_addr"`
    Proto       string              `json:"proto"`
    Headers     map[string][]string `json:"headers"`
    QueryParams map[string][]string `json:"query_params,omitempty"`
    ContentType string              `json:"content_type"`

    StatusCode      int                 `json:"status_code"`
    ResponseHeaders map[string][]string `json:"response_headers"`
    ResponseSize    int64               `json:"response_size"`

    // New fields
    Body        string `json:"body,omitempty"`         // Captured request body (text or placeholder)
    BodySize    int64  `json:"body_size,omitempty"`    // Original body size in bytes
    BodyTruncated bool `json:"body_truncated,omitempty"` // True if body was truncated
    CurlCommand string `json:"curl_command,omitempty"` // Generated curl command
}
```

#### 4.2 Request Collector Options

```go
type requestOptions struct {
    bodyCaptureEnabled  bool     // Default: false (env: PROFILER_CAPTURE_BODY)
    bodyMaxSize         int      // Default: 1048576 (env: PROFILER_BODY_MAX_SIZE)
    bodyContentTypes    []string // Default: nil (capture all text types)
    redactHeaders       bool     // Default: true (env: PROFILER_REDACT_HEADERS)
}
```

#### 4.3 Captured Body Context Value

```go
type capturedBody struct {
    content   string // Body text or "[binary data: N bytes]"
    size      int64  // Original full size
    truncated bool   // Whether max size was exceeded
    binary    bool   // Whether binary was detected
}
```

### 5. Options Design

```go
type RequestOption func(*requestOptions)

// WithBodyCapture enables or disables request body capture.
// Overrides the PROFILER_CAPTURE_BODY environment variable.
func WithBodyCapture(enabled bool) RequestOption {
    return func(o *requestOptions) { o.bodyCaptureEnabled = enabled }
}

// WithBodyMaxSize sets the maximum number of bytes to capture from the body.
// Overrides the PROFILER_BODY_MAX_SIZE environment variable.
// Default: 1048576 (1 MB).
func WithBodyMaxSize(bytes int) RequestOption {
    return func(o *requestOptions) { o.bodyMaxSize = bytes }
}

// WithBodyContentTypes restricts body capture to requests with matching
// Content-Type headers. If empty (default), all text content types are captured.
func WithBodyContentTypes(types ...string) RequestOption {
    return func(o *requestOptions) { o.bodyContentTypes = types }
}

// WithRedactHeaders enables or disables sensitive header redaction.
// Overrides the PROFILER_REDACT_HEADERS environment variable.
// Default: true (headers are redacted).
func WithRedactHeaders(enabled bool) RequestOption {
    return func(o *requestOptions) { o.redactHeaders = enabled }
}
```

### 6. Collector Lifecycle

```go
func NewRequestCollector(opts ...RequestOption) *RequestCollector {
    // 1. Start with defaults
    options := &requestOptions{
        bodyCaptureEnabled: false,
        bodyMaxSize:        1048576, // 1 MB
        bodyContentTypes:   nil,
        redactHeaders:      true,
    }

    // 2. Check environment variables (lower precedence)
    if envBool("PROFILER_CAPTURE_BODY") {
        options.bodyCaptureEnabled = true
    }
    if v := envInt("PROFILER_BODY_MAX_SIZE"); v > 0 {
        options.bodyMaxSize = v
    }
    if envBoolDefault("PROFILER_REDACT_HEADERS", true) == false {
        options.redactHeaders = false
    }

    // 3. Apply programmatic options (highest precedence)
    for _, opt := range opts {
        opt(options)
    }

    return &RequestCollector{options: options}
}

// CaptureBody reads and buffers the request body, storing it in context.
// Called by middleware BEFORE the handler runs.
func (c *RequestCollector) CaptureBody(ctx context.Context, r *http.Request) (context.Context, *http.Request) {
    if !c.options.bodyCaptureEnabled {
        return ctx, r
    }
    if r.Body == nil || r.ContentLength == 0 {
        return ctx, r
    }
    if !c.shouldCaptureContentType(r.Header.Get("Content-Type")) {
        return ctx, r
    }

    // Read body up to maxSize + 1 (to detect truncation)
    // Replace r.Body for downstream consumption
    // Store capturedBody in context
    ...
}

func (c *RequestCollector) Collect(ctx context.Context, req *http.Request, res ResponseData) (any, error) {
    // 1. Build RequestData with existing fields (method, url, headers, etc.)
    // 2. Apply header sanitization (respects redactHeaders toggle)
    // 3. Retrieve captured body from context (if present)
    // 4. Populate body fields (Body, BodySize, BodyTruncated)
    // 5. Generate curl command via BuildCurlCommand()
    // 6. Return *RequestData
}
```

### 7. Curl Command Builder

```go
// BuildCurlCommand generates a multi-line curl command from request data.
// It is a pure function with no side effects.
func BuildCurlCommand(input *CurlInput) string {
    var b strings.Builder

    // Method line (omit -X GET as it's default)
    if input.Method == "GET" || input.Method == "" {
        b.WriteString("curl")
    } else {
        b.WriteString(fmt.Sprintf("curl -X %s", input.Method))
    }

    // URL (single-quoted for shell safety)
    b.WriteString(fmt.Sprintf(" '%s'", input.URL))

    // Headers (one per line, skip excluded transport headers)
    for key, values := range input.Headers {
        if curlExcludedHeaders[key] {
            continue
        }
        for _, val := range values {
            b.WriteString(fmt.Sprintf(" \\\n  -H '%s: %s'", key, escapeSingleQuotes(val)))
        }
    }

    // Body
    if input.IsBinary {
        b.WriteString(fmt.Sprintf(" \\\n  # Body: binary data (%d bytes) - not included", input.BinarySize))
    } else if input.HasBody && input.Body != "" {
        b.WriteString(fmt.Sprintf(" \\\n  -d '%s'", escapeSingleQuotes(input.Body)))
    }

    return b.String()
}

// escapeSingleQuotes escapes single quotes for safe use in shell single-quoted strings.
// Uses the pattern: replace ' with '\'' (end quote, escaped quote, start quote).
func escapeSingleQuotes(s string) string {
    return strings.ReplaceAll(s, "'", "'\\''")
}
```

### 8. Body Reading Strategy

```go
func readBody(r *http.Request, maxSize int) (*capturedBody, *http.Request) {
    // 1. Create a LimitedReader wrapping r.Body at maxSize + 1
    limited := io.LimitReader(r.Body, int64(maxSize)+1)

    // 2. Read all bytes from limited reader
    buf, err := io.ReadAll(limited)
    if err != nil {
        // On read error, still replace body with what we got
    }

    // 3. Read any remaining bytes from r.Body into a discard counter
    //    to get the full original size (for the truncation message)
    //    Actually: we don't need full size, just whether truncation happened
    truncated := len(buf) > maxSize
    if truncated {
        buf = buf[:maxSize]
    }

    // 4. Reconstruct full body for downstream: concat buf + remaining
    //    Use io.MultiReader(bytes.NewReader(buf), r.Body) to avoid reading all
    fullBody := io.MultiReader(bytes.NewReader(buf), r.Body)
    r.Body = io.NopCloser(fullBody)

    // Wait — this is wrong. We need the FULL body for downstream.
    // Better approach: read ALL bytes (ignoring our truncation for capture),
    // then replace r.Body with a reader over ALL bytes.

    // CORRECT approach:
    // Read up to maxSize for capture, then drain rest for body restoration.
    // Store full bytes for r.Body replacement, truncated bytes for profile.

    // SIMPLEST correct approach:
    allBytes, _ := io.ReadAll(r.Body) // Read entire body
    r.Body = io.NopCloser(bytes.NewReader(allBytes)) // Restore full body

    captured := &capturedBody{
        size: int64(len(allBytes)),
    }

    if len(allBytes) > maxSize {
        captured.content = string(allBytes[:maxSize])
        captured.truncated = true
    } else {
        captured.content = string(allBytes)
    }

    return captured, r
}
```

**Final design:** Read the full body, store all bytes for restoration, but only keep up to `maxSize` bytes in the profile. This is simpler and guarantees downstream handlers get the complete body.

**Memory consideration:** For a 1MB max capture, the body buffer temporarily holds the full body in memory. Since this already happens when handlers call `io.ReadAll(r.Body)`, there's no additional memory pressure beyond what the application would use anyway.

### 9. Middleware Integration

The body capture must happen **before** the handler runs. Two approaches:

**Option A: Modify the profiler middleware directly**
```go
// In middleware.go, after context setup but before next.ServeHTTP:
if rc := p.requestCollector(); rc != nil {
    ctx, r = rc.CaptureBody(ctx, r)
    r = r.WithContext(ctx)
}
```

**Option B: RequestCollector implements ContextSetup**

The `ContextSetup` interface already exists and is called by the middleware:
```go
// SetupContext is called by the middleware before the handler.
func (c *RequestCollector) SetupContext(ctx context.Context) context.Context
```

However, `SetupContext` only receives `context.Context`, not `*http.Request`. It can't read or replace `r.Body`.

**Decision: Option A** — modify the middleware to call `CaptureBody` when the request collector has body capture enabled. This is minimal and explicit.

The middleware needs access to the `RequestCollector` specifically (not just via the generic `Collector` interface). Add a helper method to `Profiler`:

```go
func (p *Profiler) requestCollector() *RequestCollector {
    for _, c := range p.collectors {
        if rc, ok := c.(*RequestCollector); ok {
            return rc
        }
    }
    return nil
}
```

### 10. UI Panel Design

**Updated RequestPanel.vue** structure:

```
┌─────────────────────────────────────────────────────────────────┐
│ Request                                                          │
│ ┌──────────────────────────────────────────────────────────────┐│
│ │ Method       [POST]                                          ││
│ │ URL          /api/users?active=true                          ││
│ │ Host         localhost:8080                                   ││
│ │ Remote Addr  [::1]:42644                                     ││
│ │ Protocol     HTTP/1.1                                        ││
│ │ Content-Type application/json                                ││
│ └──────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────┤
│ cURL Command                                      [📋 Copy]     │
│ ┌──────────────────────────────────────────────────────────────┐│
│ │ curl -X POST 'http://localhost:8080/api/users?active=true' \ ││
│ │   -H 'Content-Type: application/json' \                      ││
│ │   -H 'Accept: application/json' \                            ││
│ │   -d '{"name":"John","email":"john@example.com"}'            ││
│ └──────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────┤
│ Request Body                        [⚠ Truncated] (collapsible) │
│ ┌──────────────────────────────────────────────────────────────┐│
│ │ {                                                            ││
│ │   "name": "John",                                           ││
│ │   "email": "john@example.com"                               ││
│ │ }                                                            ││
│ └──────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────┤
│ Query Parameters                                                 │
│ ... (existing)                                                   │
├─────────────────────────────────────────────────────────────────┤
│ Request Headers                                                  │
│ ... (existing)                                                   │
├─────────────────────────────────────────────────────────────────┤
│ Response                                                         │
│ ... (existing)                                                   │
├─────────────────────────────────────────────────────────────────┤
│ Response Headers                                                 │
│ ... (existing)                                                   │
└─────────────────────────────────────────────────────────────────┘
```

**Key UI behaviors:**
- **Curl section** appears immediately after the request info table (high visibility, always shown).
- **Copy button** uses `navigator.clipboard.writeText()` with fallback to `document.execCommand('copy')`.
- **Copy feedback:** button text changes to "Copied!" with a checkmark for 2 seconds.
- **Request Body section** appears only when `body` field is present and non-empty.
- **JSON pretty-print:** detect JSON via Content-Type, parse and format with 2-space indent.
- **Truncation badge:** yellow warning badge when `body_truncated` is true.
- **Binary placeholder:** show as an italic info message, not a code block.
- **Collapsible body:** default open for small bodies (<5KB), collapsed for large bodies.

### 11. Integration Points

1. **Middleware modification** — `middleware.go` calls `RequestCollector.CaptureBody()` before handler:
   ```go
   if rc := p.requestCollector(); rc != nil {
       ctx, r = rc.CaptureBody(ctx, r)
       r = r.WithContext(ctx)
   }
   ```

2. **Collector registration** — unchanged, users still call:
   ```go
   p.AddCollector(collector.NewRequestCollector(
       collector.WithBodyCapture(true),
       collector.WithRedactHeaders(false),
   ))
   ```

3. **Profile data** — stored in `Profile.CollectorData["request"]` as `*RequestData` (backward compatible, new fields use `omitempty`).

4. **API** — no changes needed. `GET /_profiler/api/profiles/{id}` already serves the full collector data including the new fields.

5. **UI panel** — same component name `RequestPanel`, same registration. Template extended with new sections.

### 12. Error Handling

- If `r.Body` read fails (e.g., client disconnected), body capture is silently skipped — `capturedBody` is not set in context, and `Collect()` proceeds without body data.
- If body read returns partial data before error, the partial data is still captured (useful for debugging).
- `BuildCurlCommand` never errors — it produces a best-effort command from whatever data is available.
- If the curl command would be empty (no URL), it's omitted from the output (`omitempty`).
- Missing or invalid `PROFILER_BODY_MAX_SIZE` env var value → default (1MB) is used, no error logged.
- Invalid `PROFILER_REDACT_HEADERS` value → default (true/redacted) is used.

### 13. Environment Variable Summary

| Variable | Default | Values | Description |
|----------|---------|--------|-------------|
| `PROFILER_CAPTURE_BODY` | `false` | `true`/`1` to enable | Toggle body capture |
| `PROFILER_BODY_MAX_SIZE` | `1048576` | Integer (bytes) | Max body bytes to store |
| `PROFILER_REDACT_HEADERS` | `true` | `false`/`0` to disable | Toggle header redaction |

### 14. Security Considerations

- Body capture is **opt-in only** — no body data is ever captured unless explicitly enabled.
- Header redaction is **opt-out** — sensitive headers are always redacted unless explicitly disabled.
- The curl command reflects whatever redaction state is active — if headers are redacted, the curl command shows `[REDACTED]`, making it clear the command won't work as-is without real credentials.
- Large body capture is bounded by `bodyMaxSize` — prevents OOM from multi-GB uploads.
- Body buffer is short-lived: read during middleware, consumed during `Collect()`, then eligible for GC.
