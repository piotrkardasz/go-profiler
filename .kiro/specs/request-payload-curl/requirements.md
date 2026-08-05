# Requirements: Request Payload Capture & Curl Command Generation

## Overview

Extend the go-profiler to optionally capture HTTP request bodies (payloads) in profiles and generate ready-to-use `curl` commands from captured request data. The payload capture is opt-in via an environment variable (disabled by default to avoid memory/storage overhead). The curl command generation builds on the existing request collector data — combining method, URL, headers, query parameters, and the captured body into a single copy-paste-ready `curl` command displayed in the UI. This mirrors the existing "runnable query" concept from the GORM collector but applied to HTTP requests.

## Functional Requirements

### FR-1: Request Body Capture

- FR-1.1: The request collector MUST support capturing the HTTP request body (payload).
- FR-1.2: Payload capture MUST be disabled by default (opt-in only).
- FR-1.3: Payload capture MUST be enabled by setting the environment variable `PROFILER_CAPTURE_BODY=true` (or `1`).
- FR-1.4: Payload capture MUST also be configurable programmatically via a functional option `WithBodyCapture(enabled bool)`.
- FR-1.5: When enabled, the collector MUST read and store the full request body bytes.
- FR-1.6: The collector MUST restore the request body after reading it so downstream handlers can still consume `r.Body` normally (using an `io.NopCloser` + buffered bytes).
- FR-1.7: The captured body MUST be stored as a string in the `RequestData` struct under a `body` JSON field.
- FR-1.8: When payload capture is disabled, the `body` field MUST be omitted from the JSON output (empty string or `omitempty`).
- FR-1.9: The collector MUST NOT capture bodies for requests with no body (GET, HEAD, OPTIONS, etc. — or when `ContentLength == 0`).

### FR-2: Body Size Limits

- FR-2.1: The collector MUST enforce a maximum body capture size to prevent excessive memory usage.
- FR-2.2: The default maximum body size MUST be 1 MB (1,048,576 bytes).
- FR-2.3: The maximum body size MUST be configurable via the environment variable `PROFILER_BODY_MAX_SIZE` (value in bytes).
- FR-2.4: The maximum body size MUST be configurable programmatically via `WithBodyMaxSize(bytes int)`.
- FR-2.5: When the body exceeds the maximum size, the collector MUST capture only the first N bytes and append a truncation indicator (e.g., `\n[truncated: body exceeds 1048576 bytes]`).
- FR-2.6: The full original body MUST still be available to downstream handlers regardless of truncation in the profile.

### FR-3: Body Content Type Handling

- FR-3.1: The collector MUST capture bodies for all content types when enabled (JSON, form data, XML, plain text, etc.).
- FR-3.2: For binary content types (e.g., `multipart/form-data` with file uploads, `application/octet-stream`), the collector MUST store a placeholder: `[binary data: N bytes]` instead of the raw bytes.
- FR-3.3: Binary detection MUST be based on content type header — the following MUST be treated as text (captured as-is): `application/json`, `application/xml`, `text/*`, `application/x-www-form-urlencoded`, `application/graphql`.
- FR-3.4: An option `WithBodyContentTypes(types ...string)` MUST allow users to restrict capture to specific content types only (whitelist mode).
- FR-3.5: When a content type whitelist is configured and the request's content type does not match, the body MUST NOT be captured for that request.

### FR-4: Sensitive Data Handling

- FR-4.1: The collector MUST NOT attempt to parse or mask values inside the body by default (body is treated as opaque text).
- FR-4.2: When `PROFILER_MASK_SECRETS=true` is enabled (from the config collector), the body MUST still be captured as-is — secret masking applies only to config keys, not request bodies.
- FR-4.3: A future extension point SHOULD be considered (e.g., body sanitizer function) but is NOT required for this iteration.
- FR-4.4: Sensitive header redaction (`[REDACTED]` for Authorization, Cookie, Set-Cookie) MUST be toggleable via the environment variable `PROFILER_REDACT_HEADERS` (default: `true`).
- FR-4.5: When `PROFILER_REDACT_HEADERS=false` (or `0`), sensitive headers MUST be shown in full (unredacted) in both the profile data and the generated curl command.
- FR-4.6: When `PROFILER_REDACT_HEADERS=true` (or `1`, or unset — the default), sensitive headers MUST be replaced with `[REDACTED]` as per existing behavior.
- FR-4.7: The redaction toggle MUST also be configurable programmatically via `WithRedactHeaders(enabled bool)`.
- FR-4.8: Programmatic option MUST take precedence over the environment variable.

### FR-5: Curl Command Generation

- FR-5.1: The profiler MUST generate a ready-to-use `curl` command from the captured request data.
- FR-5.2: The curl command MUST be generated server-side (in Go) and included in the profile data as a `curl_command` field in `RequestData`.
- FR-5.3: The curl command MUST include: the HTTP method (`-X METHOD`), the full URL (scheme + host + path + query string), all request headers (`-H "Header: value"` for each), and the request body (`-d 'body'`) when captured.
- FR-5.4: The curl command MUST properly escape single quotes in header values and body content for shell safety.
- FR-5.5: Sensitive headers (Authorization, Cookie) MUST respect the `PROFILER_REDACT_HEADERS` setting (FR-4.4) — when redaction is enabled, use `[REDACTED]`; when disabled, include the full header value in the curl command.
- FR-5.6: The curl command MUST be generated only when the request panel data is collected (every profiled request gets a curl command, regardless of whether body capture is enabled).
- FR-5.7: When body capture is disabled, the curl command MUST still be generated using method, URL, and headers (without `-d`).
- FR-5.8: When the body was truncated (FR-2.5), the curl command MUST use the truncated body content (what's stored in the profile).
- FR-5.9: For binary bodies (FR-3.2), the curl command MUST omit the `-d` flag and include a comment: `# Body: binary data (N bytes) - not included`.
- FR-5.10: The curl command MUST reconstruct the full URL using the request's scheme (defaulting to `http` if not detectable), host, path, and raw query.

### FR-6: Curl Command Formatting

- FR-6.1: The curl command MUST use multi-line format with backslash line continuations for readability.
- FR-6.2: Each header MUST be on its own line.
- FR-6.3: The body (`-d`) MUST be on its own line.
- FR-6.4: Example format:
  ```
  curl -X POST 'http://localhost:8080/api/users?active=true' \
    -H 'Content-Type: application/json' \
    -H 'Accept: application/json' \
    -H 'Authorization: [REDACTED]' \
    -d '{"name":"John","email":"john@example.com"}'
  ```
- FR-6.5: For GET requests without a body, the command MUST be simplified (omit `-X GET` as it's the default, omit `-d`).
- FR-6.6: Headers that are purely transport-level (e.g., `Content-Length`, `Accept-Encoding`, `Connection`, `Host`) SHOULD be excluded from the curl command to keep it clean and focused on application-level headers.

### FR-7: UI - Request Panel Updates

- FR-7.1: The `RequestPanel` component MUST display the captured request body when present.
- FR-7.2: The body MUST be displayed in a collapsible section labeled "Request Body".
- FR-7.3: For JSON bodies, the body MUST be syntax-highlighted and pretty-printed.
- FR-7.4: For non-JSON bodies, the body MUST be displayed as preformatted text.
- FR-7.5: The truncation indicator (FR-2.5) MUST be visually distinct (e.g., warning badge).
- FR-7.6: The binary placeholder (FR-3.2) MUST be displayed as an informational message.

### FR-8: UI - Curl Command Display

- FR-8.1: The `RequestPanel` component MUST display the generated curl command in a dedicated section.
- FR-8.2: The curl command section MUST be labeled "cURL Command" or "Curl".
- FR-8.3: The curl command MUST be displayed in a code block with monospace font.
- FR-8.4: A "Copy" button MUST be provided that copies the curl command to the clipboard.
- FR-8.5: The copy button MUST provide visual feedback on successful copy (e.g., checkmark icon, tooltip change).
- FR-8.6: The curl command MUST always be visible (not collapsed by default) since it's the primary action item.
- FR-8.7: The curl command section MUST appear even when body capture is disabled (it will just lack the `-d` flag).

### FR-9: Configuration Options

- FR-9.1: The request collector MUST accept functional options for payload capture configuration.
- FR-9.2: Options MUST include:
  - `WithBodyCapture(enabled bool)` — enable/disable body capture (overrides env var)
  - `WithBodyMaxSize(bytes int)` — set max body capture size
  - `WithBodyContentTypes(types ...string)` — whitelist content types for capture
  - `WithRedactHeaders(enabled bool)` — enable/disable sensitive header redaction (overrides env var)
- FR-9.3: Environment variables MUST be checked as fallback when programmatic options are not set.
- FR-9.4: Programmatic options MUST take precedence over environment variables.
- FR-9.5: Environment variables for this feature:
  - `PROFILER_CAPTURE_BODY` — `true`/`1` to enable body capture
  - `PROFILER_BODY_MAX_SIZE` — max bytes (integer string, e.g., `2097152` for 2MB)
  - `PROFILER_REDACT_HEADERS` — `true`/`1` (default) to redact sensitive headers, `false`/`0` to show full values

## Non-Functional Requirements

### NFR-1: Performance

- NFR-1.1: When body capture is disabled (default), the collector MUST NOT read or buffer the request body — zero overhead.
- NFR-1.2: When body capture is enabled, the body MUST be read once and buffered efficiently (single allocation where possible).
- NFR-1.3: Body capture MUST NOT introduce a measurable latency increase beyond the I/O of reading the body bytes (which would happen anyway in the handler).
- NFR-1.4: Curl command generation MUST be fast (<1ms) and allocation-efficient (use `strings.Builder`).
- NFR-1.5: The body buffer MUST be released after the profile is stored (no long-lived references).

### NFR-2: Compatibility

- NFR-2.1: MUST work with Go 1.21+ (matching root module requirement).
- NFR-2.2: MUST NOT break existing `RequestCollector` behavior when body capture is not enabled.
- NFR-2.3: MUST NOT interfere with request body consumption by downstream handlers.
- NFR-2.4: MUST work with all HTTP methods and content types.
- NFR-2.5: The `RequestData` struct changes MUST be backward-compatible (new fields use `omitempty`).

### NFR-3: Storage

- NFR-3.1: Profile JSON size increase MUST be bounded by the max body size setting.
- NFR-3.2: The curl command adds minimal overhead (formatted copy of existing data).
- NFR-3.3: Large bodies stored in profiles MUST NOT cause performance issues in the UI (lazy rendering / virtual scroll for very large bodies is acceptable as a future enhancement).

### NFR-4: Security

- NFR-4.1: The body capture feature MUST be opt-in only — never enabled by default.
- NFR-4.2: Sensitive header redaction MUST be toggleable — enabled by default for safety, but switchable via env var for full debugging workflows.
- NFR-4.3: The collector MUST NOT log or print captured body content outside of the profile storage.
- NFR-4.4: The feature documentation MUST warn users that enabling body capture may store sensitive data (passwords, tokens in POST bodies) in profiles.

### NFR-5: Extensibility

- NFR-5.1: The curl generation logic MUST be in a separate, testable function (not embedded in the collector's Collect method).
- NFR-5.2: Future enhancements (body sanitization, response body capture, HAR export) MUST be possible without breaking changes.
- NFR-5.3: The content type detection logic MUST be easily extensible (simple list/map lookup).
