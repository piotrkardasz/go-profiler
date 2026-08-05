# Tasks: Request Payload Capture & Curl Command Generation

## Implementation Tasks

### Task 1: Add functional options to RequestCollector

**Objective:** Transform `RequestCollector` from a zero-field struct into a configurable collector with functional options, reading environment variables as defaults.

**Implementation:**
- `requestOptions` struct: `bodyCaptureEnabled bool`, `bodyMaxSize int`, `bodyContentTypes []string`, `redactHeaders bool`
- `RequestOption` type: `func(*requestOptions)`
- Option functions:
  - `WithBodyCapture(enabled bool)` — override `PROFILER_CAPTURE_BODY`
  - `WithBodyMaxSize(bytes int)` — override `PROFILER_BODY_MAX_SIZE`
  - `WithBodyContentTypes(types ...string)` — whitelist content types
  - `WithRedactHeaders(enabled bool)` — override `PROFILER_REDACT_HEADERS`
- Environment variable helpers: `envBool(key string) bool`, `envInt(key string) int`, `envBoolDefault(key string, def bool) bool`
- Defaults: body capture off, max size 1MB, redact headers on, no content type whitelist
- Precedence: programmatic options > env vars > defaults
- Update `NewRequestCollector(opts ...RequestOption)` signature (backward-compatible, zero args = old behavior)
- Update `RequestCollector` struct to hold `*requestOptions`

**Files to create:**
- `collector/request_options.go`

**Files to modify:**
- `collector/request.go` — change struct definition, update constructor

---

### Task 2: Implement header redaction toggle

**Objective:** Make sensitive header redaction switchable via env var and option, updating the existing `sanitizeHeaders` function.

**Implementation:**
- Move `sanitizeHeaders` from a package-level function to a method on `RequestCollector` so it can access `options.redactHeaders`
- When `redactHeaders` is `true` (default): existing behavior (Authorization, Cookie, Set-Cookie → `[REDACTED]`)
- When `redactHeaders` is `false`: pass all header values through unmodified
- Update `Collect()` to call the method-based version
- Constants for env var name: `EnvRedactHeaders = "PROFILER_REDACT_HEADERS"`

**Files to modify:**
- `collector/request.go` — `sanitizeHeaders` becomes a method, `Collect()` updated
- `collector/request_options.go` — env var constant

---

### Task 3: Implement request body capture

**Objective:** Read and buffer the request body before the handler runs, store it in context for later collection.

**Implementation:**
- Context key type: `type bodyContextKey struct{}`
- Context value: `capturedBody` struct with `content string`, `size int64`, `truncated bool`, `binary bool`
- `CaptureBody(ctx context.Context, r *http.Request) (context.Context, *http.Request)`:
  - Return early if body capture disabled, `r.Body == nil`, or `r.ContentLength == 0`
  - Check content type against whitelist (if configured) — skip if no match
  - Detect binary content type via `isTextContentType(contentType string) bool`
  - Read full body via `io.ReadAll(r.Body)`
  - Replace `r.Body` with `io.NopCloser(bytes.NewReader(allBytes))`
  - If binary: set `binary = true`, `content = "[binary data: N bytes]"`
  - If exceeds max size: truncate to `maxSize` bytes, set `truncated = true`
  - Store `capturedBody` in context
  - Return enriched context and modified request
- `bodyFromContext(ctx context.Context) *capturedBody` — retrieves stored body (nil if not captured)
- `isTextContentType(ct string) bool` — checks against whitelist (json, xml, form, graphql, text/*)
- `shouldCaptureContentType(ct string) bool` — checks user whitelist if configured
- Export `CaptureBody` so middleware can call it

**Files to create:**
- `collector/request_body.go`

---

### Task 4: Implement curl command builder

**Objective:** Create a pure function that generates a multi-line curl command from request data.

**Implementation:**
- `CurlInput` struct: `Method string`, `URL string`, `Headers map[string][]string`, `Body string`, `HasBody bool`, `IsBinary bool`, `BinarySize int64`
- `BuildCurlCommand(input *CurlInput) string`:
  - Use `strings.Builder` for efficient string construction
  - Omit `-X GET` for GET requests (curl default)
  - Single-quote the URL
  - Iterate headers alphabetically (deterministic output), skip transport-level headers
  - Each header on its own line with `\` continuation: `-H 'Key: Value'`
  - Body on its own line: `-d 'body content'`
  - Binary comment: `# Body: binary data (N bytes) - not included`
  - Handle empty/missing body gracefully (no `-d` line)
- `escapeSingleQuotes(s string) string` — replaces `'` with `'\''`
- Transport header exclusion set: `Content-Length`, `Accept-Encoding`, `Connection`, `Host`, `Transfer-Encoding`, `Upgrade`
- Sort header keys before iterating for deterministic output

**Files to create:**
- `collector/request_curl.go`

---

### Task 5: Integrate body and curl into RequestCollector.Collect()

**Objective:** Wire body retrieval from context and curl generation into the existing `Collect()` method.

**Implementation:**
- In `Collect()`, after building base `RequestData`:
  - Call `bodyFromContext(ctx)` to get captured body
  - If body present: set `data.Body`, `data.BodySize`, `data.BodyTruncated`
  - Build `CurlInput` from the collected data:
    - Reconstruct full URL: scheme (detect from TLS/proto or default `http`) + host + path + query
    - Use sanitized headers (post-redaction)
    - Set `HasBody` if body was captured and not binary
    - Set `IsBinary` and `BinarySize` from captured body metadata
  - Call `BuildCurlCommand(&input)` → set `data.CurlCommand`
- Add `RequestData` fields: `Body string`, `BodySize int64`, `BodyTruncated bool`, `CurlCommand string` (all `json:",omitempty"`)
- URL reconstruction helper: `buildFullURL(req *http.Request) string` — combines scheme + host + path + raw query

**Files to modify:**
- `collector/request.go` — extend `RequestData` struct, update `Collect()` method

---

### Task 6: Integrate body capture into profiler middleware

**Objective:** Call `RequestCollector.CaptureBody()` in the middleware before the handler executes.

**Implementation:**
- Add `requestCollector() *RequestCollector` helper method on `Profiler`:
  - Iterate `p.collectors`, find and return `*RequestCollector` via type assertion
  - Cache result after first lookup (optional optimization)
- In `Middleware()`, after context setup (after `ContextSetup` loop) but before `next.ServeHTTP`:
  ```go
  if rc := p.requestCollector(); rc != nil && rc.BodyCaptureEnabled() {
      ctx, r = rc.CaptureBody(ctx, r)
      r = r.WithContext(ctx)
  }
  ```
- Add `BodyCaptureEnabled() bool` exported method on `RequestCollector` for the middleware check
- Ensure body capture only happens when profiler is enabled and request is not skipped (sampling, route prefix)

**Files to modify:**
- `middleware.go` — add body capture call before handler
- `profiler.go` — add `requestCollector()` helper method

---

### Task 7: Write unit tests for body capture

**Objective:** Test body reading, content type detection, truncation, and binary handling.

**Tests:**
- `TestCaptureBodyDisabled`: returns original context/request unchanged when disabled
- `TestCaptureBodyNilBody`: no-op when r.Body is nil
- `TestCaptureBodyZeroContentLength`: no-op when ContentLength is 0
- `TestCaptureBodyJSON`: captures JSON body correctly, downstream handler can still read full body
- `TestCaptureBodyFormData`: captures url-encoded form data
- `TestCaptureBodyTruncation`: body exceeding max size is truncated, full body still available downstream
- `TestCaptureBodyBinaryDetection`: binary content types produce placeholder
- `TestCaptureBodyContentTypeWhitelist`: only matching types captured
- `TestCaptureBodyContentTypeWhitelistMiss`: non-matching type skipped
- `TestIsTextContentType`: table-driven tests for content type classification
- `TestBodyFromContextNil`: returns nil when no body in context
- `TestCaptureBodyDownstreamReadable`: handler can read full body after capture (integration)

**Files to create:**
- `collector/request_body_test.go`

---

### Task 8: Write unit tests for curl command builder

**Objective:** Test curl generation with various input combinations.

**Tests:**
- `TestBuildCurlCommandGET`: simple GET without body, no `-X GET`
- `TestBuildCurlCommandPOSTWithBody`: POST with JSON body and headers
- `TestBuildCurlCommandPUTWithBody`: PUT method shown explicitly
- `TestBuildCurlCommandNoBody`: POST without body capture (no `-d` line)
- `TestBuildCurlCommandBinaryBody`: binary body produces comment
- `TestBuildCurlCommandRedactedHeaders`: `[REDACTED]` values pass through
- `TestBuildCurlCommandTransportHeadersExcluded`: Content-Length, Host etc. not in output
- `TestBuildCurlCommandSingleQuoteEscaping`: body and headers with single quotes escaped
- `TestBuildCurlCommandURLWithQueryParams`: URL includes query string
- `TestBuildCurlCommandMultipleHeaders`: multiple values for same header key
- `TestBuildCurlCommandHeadersSorted`: headers appear in alphabetical order
- `TestBuildCurlCommandEmptyURL`: graceful handling of edge case
- `TestEscapeSingleQuotes`: table-driven test for escape function

**Files to create:**
- `collector/request_curl_test.go`

---

### Task 9: Write unit tests for options and redaction toggle

**Objective:** Test functional options, env var parsing, and header redaction toggle behavior.

**Tests:**
- `TestRequestCollectorDefaultOptions`: verify defaults (body off, redact on, 1MB max)
- `TestRequestCollectorWithBodyCapture`: option enables body capture
- `TestRequestCollectorWithBodyMaxSize`: option sets max size
- `TestRequestCollectorWithBodyContentTypes`: option sets whitelist
- `TestRequestCollectorWithRedactHeaders`: option disables redaction
- `TestRequestCollectorEnvVarBodyCapture`: `PROFILER_CAPTURE_BODY=true` enables capture
- `TestRequestCollectorEnvVarMaxSize`: `PROFILER_BODY_MAX_SIZE=2097152` sets size
- `TestRequestCollectorEnvVarRedactHeaders`: `PROFILER_REDACT_HEADERS=false` disables redaction
- `TestRequestCollectorOptionOverridesEnv`: programmatic option beats env var
- `TestRequestCollectorRedactHeadersTrue`: Authorization, Cookie, Set-Cookie redacted
- `TestRequestCollectorRedactHeadersFalse`: all headers shown in full
- `TestRequestCollectorBackwardCompatible`: `NewRequestCollector()` with no args works as before

**Files to modify:**
- `collector/request_test.go` — add new test cases alongside existing tests

---

### Task 10: Update RequestPanel.vue with curl command section

**Objective:** Add the curl command display with a copy button to the request panel UI.

**Implementation:**
- **Curl section** (always shown when `curl_command` is present):
  - Section header: "cURL Command" with a "Copy" button aligned right
  - Code block (`<pre><code>`) displaying the multi-line curl command
  - Copy button uses `navigator.clipboard.writeText()`
  - Copy feedback: button text changes to "Copied ✓" for 2 seconds (use `ref` + `setTimeout`)
  - Fallback for older browsers: create temporary textarea, `execCommand('copy')`
- **TypeScript interface update**: add `body`, `body_size`, `body_truncated`, `curl_command` to `RequestData`
- **Styling**: code block with dark background (`#1e1e2e`), monospace font, padding, border-radius
- **Copy button styling**: small button, positioned top-right of code block, subtle background

**Files to modify:**
- `ui/src/components/panels/RequestPanel.vue`

---

### Task 11: Update RequestPanel.vue with body display section

**Objective:** Add request body display with JSON pretty-printing and truncation indicator.

**Implementation:**
- **Body section** (shown only when `body` field is present and non-empty):
  - Section header: "Request Body"
  - Truncation badge: yellow warning badge when `body_truncated` is true, showing "Truncated (original: X bytes)"
  - Binary placeholder: italic info message when body starts with `[binary data:`
  - JSON detection: if `content_type` contains `json`, attempt `JSON.parse()` then `JSON.stringify(null, 2)` for pretty-print
  - JSON display: `<pre><code>` with syntax highlighting (basic: strings green, numbers blue, keys white, nulls gray)
  - Non-JSON display: `<pre><code>` as plain preformatted text
  - Collapsible: default expanded for bodies < 5KB, collapsed for larger with "Show body (X KB)" toggle
- **Computed properties**:
  - `hasBody`: checks if `body` field exists and is non-empty
  - `isJsonBody`: checks content_type for json
  - `formattedBody`: pretty-prints JSON or returns raw text
  - `isBinaryPlaceholder`: checks if body starts with `[binary data:`
  - `bodyCollapsed`: reactive ref, default based on body size

**Files to modify:**
- `ui/src/components/panels/RequestPanel.vue`

---

### Task 12: Update basic example to demonstrate body capture

**Objective:** Show payload capture and curl generation in the basic example.

**Implementation:**
- Add `PROFILER_CAPTURE_BODY=true` to `examples/basic/.env`
- Add `PROFILER_REDACT_HEADERS=false` to `examples/basic/.env` (local dev, show full headers)
- Add a POST endpoint to `examples/basic/main.go` that accepts a JSON body (e.g., `/api/echo`)
- Add comments in the example explaining the env vars and their effect
- Optionally add `WithBodyCapture(true)` as a commented-out programmatic alternative

**Files to modify:**
- `examples/basic/.env` — add new env vars
- `examples/basic/main.go` — add POST endpoint demonstrating body capture

---

### Task 13: Final verification

**Objective:** Verify all components build, test, and integrate correctly.

**Verification steps:**
- `go build ./...` — root module builds without errors
- `go test ./collector/...` — all collector tests pass (existing + new)
- `go test ./...` — all tests pass including profiler/middleware tests
- `go vet ./...` — no warnings
- GORM collector module still builds: `cd collector/gorm && go build ./...`
- UI TypeScript check: `cd ui && npx vue-tsc --noEmit`
- UI Vite build: `cd ui && npx vite build`
- Verify backward compatibility: `NewRequestCollector()` with no args produces same output as before (no body, no curl if body capture off — but curl IS always generated from method+URL+headers)
- Verify `PROFILER_CAPTURE_BODY=true` enables body in profile JSON
- Verify `PROFILER_REDACT_HEADERS=false` shows full header values
- Verify curl command is present in profile JSON for all requests
- Verify no external dependencies added to root `go.mod`

---
