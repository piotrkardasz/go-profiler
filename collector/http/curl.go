package http

import (
	"fmt"
	"sort"
	"strings"
)

// transportHeaders are headers excluded from cURL commands as they are
// managed by the transport layer.
var transportHeaders = map[string]bool{
	"content-length":    true,
	"host":             true,
	"accept-encoding":  true,
	"connection":       true,
	"transfer-encoding": true,
	"upgrade":          true,
}

// buildCurlCommand generates a cURL command string for a captured HTTP call entry.
func buildCurlCommand(entry *HTTPCallEntry, opts *options) string {
	var b strings.Builder

	// Start with curl command
	if entry.Method != "GET" && entry.Method != "" {
		fmt.Fprintf(&b, "curl -X %s", entry.Method)
	} else {
		b.WriteString("curl")
	}

	// URL
	fmt.Fprintf(&b, " '%s'", escapeSingleQuotes(entry.URL))

	// Headers (sorted for deterministic output)
	if entry.RequestHeaders != nil {
		keys := make([]string, 0, len(entry.RequestHeaders))
		for k := range entry.RequestHeaders {
			if transportHeaders[strings.ToLower(k)] {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			values := entry.RequestHeaders[k]
			for _, v := range values {
				// Skip redacted headers in curl output
				if opts != nil && opts.redactHeaders[strings.ToLower(k)] {
					continue
				}
				fmt.Fprintf(&b, " -H '%s: %s'", escapeSingleQuotes(k), escapeSingleQuotes(v))
			}
		}
	}

	// Body
	if entry.RequestBody != "" {
		fmt.Fprintf(&b, " -d '%s'", escapeSingleQuotes(entry.RequestBody))
	}

	return b.String()
}

// escapeSingleQuotes escapes single quotes for safe shell use.
// Replaces ' with '\'' (end quote, escaped quote, start quote).
func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}
