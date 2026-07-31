package profiler

import (
	"net/http"
	"strings"
	"time"

	"github.com/piotrkardasz/go-profiler/collector"
)

const (
	// HeaderProfilerID is the response header containing the profile token.
	HeaderProfilerID = "X-Profiler-Id"
)

// responseWriter wraps http.ResponseWriter to capture the status code,
// response headers, and body size.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int64
	written    bool
}

// newResponseWriter creates a new responseWriter wrapping the given writer.
func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // default if WriteHeader is never called
	}
}

// WriteHeader captures the status code and delegates to the wrapped writer.
func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures the body size and delegates to the wrapped writer.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.written = true
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.size += int64(n)
	return n, err
}

// Flush implements http.Flusher if the underlying writer supports it.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter for http.ResponseController compatibility.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Middleware returns an HTTP middleware that profiles each request.
// It wraps the response writer to capture status/size, runs all registered
// collectors after the handler completes, sets the X-Profiler-Id header,
// and stores the profile asynchronously.
func (p *Profiler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip if profiler is disabled
		if !p.IsEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		// Skip profiler's own routes to avoid self-profiling
		cfg := p.Config()
		if strings.HasPrefix(r.URL.Path, cfg.RoutePrefix) {
			next.ServeHTTP(w, r)
			return
		}

		// Generate profile ID upfront so we can set the header before writing
		profileID, err := GenerateProfileID()
		if err != nil {
			p.logger.Error("failed to generate profile ID", "error", err.Error())
			next.ServeHTTP(w, r)
			return
		}

		// Reset collectors for this request
		p.ResetCollectors()

		// Set up context with timing and memory snapshots
		startTime := time.Now()
		ctx := r.Context()
		ctx = collector.WithStartTime(ctx, startTime)
		ctx = collector.WithMemoryStats(ctx)

		// Let collectors that implement ContextSetup inject their values
		// into the context before the handler runs.
		for _, c := range p.Collectors() {
			if cs, ok := c.(collector.ContextSetup); ok {
				ctx = cs.SetupContext(ctx)
			}
		}

		r = r.WithContext(ctx)

		// Set the profiler ID header before the handler writes the response
		w.Header().Set(HeaderProfilerID, profileID)

		// Wrap the response writer to capture status code and size
		wrapped := newResponseWriter(w)

		// Execute the actual handler
		next.ServeHTTP(wrapped, r)

		// Compute duration
		duration := time.Since(startTime)

		// Build response data for collectors
		resData := collector.ResponseData{
			StatusCode: wrapped.statusCode,
			Headers:    wrapped.Header(),
			Size:       wrapped.size,
		}

		// Collect profile data
		profile := p.CollectProfile(ctx, r, resData)
		profile.ID = profileID
		profile.Method = r.Method
		profile.URL = r.URL.String()
		profile.StatusCode = wrapped.statusCode
		profile.Timestamp = startTime
		profile.Duration = duration

		// Store profile and run late collectors asynchronously
		go func() {
			// Run late collectors
			lateData := p.CollectLate(ctx)
			for name, data := range lateData {
				profile.CollectorData[name] = data
			}

			// Persist
			if storage := p.Storage(); storage != nil {
				if err := storage.Store(profile); err != nil {
					p.logger.Error("failed to store profile",
						"profile_id", profileID,
						"error", err.Error(),
					)
				}
			}
		}()
	})
}
