package gormcollector

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// contextKey is used to store query data in the request context.
type contextKeyType struct{}

var contextKey = contextKeyType{}

// txKeyType is the context key for tracking the current transaction ID.
type txKeyType struct{}

var txKey = txKeyType{}

// requestQueries holds accumulated queries for a single HTTP request.
type requestQueries struct {
	mu      sync.Mutex
	queries []QueryEntry
	index   int
}

// Plugin implements gorm.Plugin to intercept database queries.
type Plugin struct {
	connectionName   string
	backtraceEnabled bool
}

// newPlugin creates a GORM plugin for the given connection name.
func newPlugin(connectionName string, backtraceEnabled bool) *Plugin {
	return &Plugin{
		connectionName:   connectionName,
		backtraceEnabled: backtraceEnabled,
	}
}

// Name returns the plugin name (required by gorm.Plugin interface).
func (p *Plugin) Name() string {
	return "go-profiler:gorm-collector:" + p.connectionName
}

// Initialize registers the plugin's callbacks with GORM.
func (p *Plugin) Initialize(db *gorm.DB) error {
	// Register "before" callbacks to record start time
	cb := db.Callback()

	if err := cb.Create().Before("gorm:create").Register("profiler:before_create", p.before); err != nil {
		return fmt.Errorf("gormcollector: register before_create: %w", err)
	}
	if err := cb.Query().Before("gorm:query").Register("profiler:before_query", p.before); err != nil {
		return fmt.Errorf("gormcollector: register before_query: %w", err)
	}
	if err := cb.Update().Before("gorm:update").Register("profiler:before_update", p.before); err != nil {
		return fmt.Errorf("gormcollector: register before_update: %w", err)
	}
	if err := cb.Delete().Before("gorm:delete").Register("profiler:before_delete", p.before); err != nil {
		return fmt.Errorf("gormcollector: register before_delete: %w", err)
	}
	if err := cb.Raw().Before("gorm:raw").Register("profiler:before_raw", p.before); err != nil {
		return fmt.Errorf("gormcollector: register before_raw: %w", err)
	}
	if err := cb.Row().Before("gorm:row").Register("profiler:before_row", p.before); err != nil {
		return fmt.Errorf("gormcollector: register before_row: %w", err)
	}

	// Register "after" callbacks to capture query details
	if err := cb.Create().After("gorm:create").Register("profiler:after_create", p.after("INSERT")); err != nil {
		return fmt.Errorf("gormcollector: register after_create: %w", err)
	}
	if err := cb.Query().After("gorm:query").Register("profiler:after_query", p.after("SELECT")); err != nil {
		return fmt.Errorf("gormcollector: register after_query: %w", err)
	}
	if err := cb.Update().After("gorm:update").Register("profiler:after_update", p.after("UPDATE")); err != nil {
		return fmt.Errorf("gormcollector: register after_update: %w", err)
	}
	if err := cb.Delete().After("gorm:delete").Register("profiler:after_delete", p.after("DELETE")); err != nil {
		return fmt.Errorf("gormcollector: register after_delete: %w", err)
	}
	if err := cb.Raw().After("gorm:raw").Register("profiler:after_raw", p.after("RAW")); err != nil {
		return fmt.Errorf("gormcollector: register after_raw: %w", err)
	}
	if err := cb.Row().After("gorm:row").Register("profiler:after_row", p.after("RAW")); err != nil {
		return fmt.Errorf("gormcollector: register after_row: %w", err)
	}

	return nil
}

// startTimeKey stores query start time in GORM's statement settings.
const startTimeKey = "profiler:start_time"

// before is called before query execution to record the start time.
func (p *Plugin) before(db *gorm.DB) {
	if db.Statement == nil {
		return
	}
	db.InstanceSet(startTimeKey, time.Now())
}

// after returns a callback function for the given operation type.
func (p *Plugin) after(operation string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		if db.Statement == nil {
			return
		}

		// Get the request queries from context
		ctx := db.Statement.Context
		rq := queriesFromContext(ctx)
		if rq == nil {
			// Not within a profiled request, skip
			return
		}

		// Calculate duration
		var durationMs float64
		if startVal, ok := db.InstanceGet(startTimeKey); ok {
			if startTime, ok := startVal.(time.Time); ok {
				durationMs = float64(time.Since(startTime).Microseconds()) / 1000.0
			}
		}

		// Build the query entry
		sql := db.Dialector.Explain(db.Statement.SQL.String(), db.Statement.Vars...)
		entry := QueryEntry{
			SQL:           db.Statement.SQL.String(),
			Params:        cloneParams(db.Statement.Vars),
			RunnableQuery: sql,
			DurationMs:    durationMs,
			RowsAffected:  db.RowsAffected,
			Operation:     operation,
			Connection:    p.connectionName,
			Timestamp:     time.Now(),
		}

		// Capture error if present
		if db.Error != nil && db.Error != gorm.ErrRecordNotFound {
			entry.Error = db.Error.Error()
		}

		// Capture transaction ID from context
		if txID, ok := ctx.Value(txKey).(string); ok {
			entry.TransactionID = txID
		}

		// Capture backtrace if enabled
		if p.backtraceEnabled {
			entry.Backtrace = captureBacktrace()
		}

		// Add to request queries
		rq.mu.Lock()
		entry.Index = rq.index
		rq.index++
		rq.queries = append(rq.queries, entry)
		rq.mu.Unlock()
	}
}

// WithContext returns a new context with query tracking initialized.
// This should be called at the start of each HTTP request (typically by the collector's middleware).
// If query tracking is already active in the context, it returns the context unchanged.
func WithContext(ctx context.Context) context.Context {
	if rq := queriesFromContext(ctx); rq != nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey, &requestQueries{
		queries: make([]QueryEntry, 0, 16),
	})
}

// queriesFromContext retrieves the request query tracker from context.
func queriesFromContext(ctx context.Context) *requestQueries {
	if rq, ok := ctx.Value(contextKey).(*requestQueries); ok {
		return rq
	}
	return nil
}

// QueriesFromContext retrieves all captured queries from the context.
// Returns nil if no query tracking is active.
func QueriesFromContext(ctx context.Context) []QueryEntry {
	rq := queriesFromContext(ctx)
	if rq == nil {
		return nil
	}
	rq.mu.Lock()
	defer rq.mu.Unlock()
	result := make([]QueryEntry, len(rq.queries))
	copy(result, rq.queries)
	return result
}

// WithTransaction returns a context with a new transaction ID.
// Use this when beginning a transaction to group queries.
func WithTransaction(ctx context.Context) context.Context {
	return context.WithValue(ctx, txKey, generateTxID())
}

// TransactionIDFromContext retrieves the current transaction ID from context.
func TransactionIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(txKey).(string)
	return id, ok
}

// generateTxID generates a short unique transaction identifier.
func generateTxID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("tx-%d", time.Now().UnixNano())
	}
	return "tx-" + hex.EncodeToString(b)
}

// cloneParams creates a shallow copy of query parameters, converting
// non-serializable types to their string representation.
func cloneParams(vars []any) []any {
	if len(vars) == 0 {
		return nil
	}
	params := make([]any, len(vars))
	for i, v := range vars {
		switch val := v.(type) {
		case time.Time:
			params[i] = val.Format(time.RFC3339Nano)
		case *time.Time:
			if val != nil {
				params[i] = val.Format(time.RFC3339Nano)
			} else {
				params[i] = nil
			}
		case []byte:
			if len(val) > 64 {
				params[i] = fmt.Sprintf("[%d bytes]", len(val))
			} else {
				params[i] = fmt.Sprintf("%x", val)
			}
		default:
			params[i] = v
		}
	}
	return params
}

// captureBacktrace captures the Go call stack, filtering out runtime/gorm internals.
func captureBacktrace() []string {
	const maxDepth = 32
	pcs := make([]uintptr, maxDepth)
	n := runtime.Callers(4, pcs) // skip Callers, captureBacktrace, after, gorm internals
	if n == 0 {
		return nil
	}

	frames := runtime.CallersFrames(pcs[:n])
	var trace []string

	for {
		frame, more := frames.Next()

		// Skip gorm internals and this package
		if isInternalFrame(frame.Function) {
			if !more {
				break
			}
			continue
		}

		trace = append(trace, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))

		if len(trace) >= 10 || !more {
			break
		}
	}

	return trace
}

// isInternalFrame returns true if the frame belongs to gorm or this package.
func isInternalFrame(function string) bool {
	return strings.Contains(function, "gorm.io/gorm") ||
		strings.Contains(function, "gormcollector") ||
		strings.Contains(function, "runtime.") ||
		strings.Contains(function, "database/sql")
}
