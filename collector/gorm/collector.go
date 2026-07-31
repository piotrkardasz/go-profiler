package gormcollector

import (
	"context"
	"net/http"

	"github.com/piotrkardasz/go-profiler/collector"
)

// Collector implements the profiler's collector.Collector interface for GORM.
// It aggregates query data captured during an HTTP request and produces
// structured profiling output with analysis.
type Collector struct {
	opts *Options
}

// New creates a new GORM collector with the given options.
// It registers the profiler plugin with each configured GORM connection.
//
// Example usage:
//
//	gormCollector, err := gormcollector.New(
//	    gormcollector.WithConnection("postgres-main", postgresDB),
//	    gormcollector.WithConnection("mysql-analytics", mysqlDB),
//	    gormcollector.WithSlowThreshold(200 * time.Millisecond),
//	    gormcollector.WithN1Threshold(3),
//	)
func New(options ...Option) (*Collector, error) {
	opts := defaultOptions()
	for _, o := range options {
		o(opts)
	}

	backtraceEnabled := opts.isBacktraceEnabled()

	// Register the GORM plugin with each connection
	for _, conn := range opts.Connections {
		plugin := newPlugin(conn.Name, backtraceEnabled)
		if err := conn.DB.Use(plugin); err != nil {
			return nil, err
		}
	}

	return &Collector{opts: opts}, nil
}

// Name returns the collector identifier.
func (c *Collector) Name() string {
	return "gorm"
}

// Collect gathers all query data from the request context, performs analysis,
// and returns the structured GormData.
func (c *Collector) Collect(ctx context.Context, _ *http.Request, _ collector.ResponseData) (any, error) {
	queries := QueriesFromContext(ctx)
	if queries == nil {
		return &GormData{
			Connections: []ConnectionData{},
			Summary: Summary{
				QueriesPerConnection: make(map[string]int),
			},
		}, nil
	}

	// Group queries by connection
	connectionMap := make(map[string]*ConnectionData)
	connectionOrder := make([]string, 0)

	for _, q := range queries {
		conn, exists := connectionMap[q.Connection]
		if !exists {
			conn = &ConnectionData{
				Name:          q.Connection,
				Queries:       make([]QueryEntry, 0),
				Transactions:  make([]TransactionGroup, 0),
				FailedQueries: make([]QueryEntry, 0),
			}
			connectionMap[q.Connection] = conn
			connectionOrder = append(connectionOrder, q.Connection)
		}

		conn.Queries = append(conn.Queries, q)
		conn.TotalDurationMs += q.DurationMs
		conn.QueryCount++

		if q.Error != "" {
			conn.FailedQueries = append(conn.FailedQueries, q)
		}
	}

	// Build transaction groups per connection
	for _, conn := range connectionMap {
		conn.Transactions = buildTransactionGroups(conn.Queries)
	}

	// Build ordered connections slice
	connections := make([]ConnectionData, 0, len(connectionOrder))
	for _, name := range connectionOrder {
		connections = append(connections, *connectionMap[name])
	}

	// Run analysis
	analysis := analyze(queries, c.opts)

	// Collect all failed queries
	var failedQueries []QueryEntry
	for _, conn := range connections {
		failedQueries = append(failedQueries, conn.FailedQueries...)
	}

	// Build summary
	summary := buildSummary(connections, analysis)

	return &GormData{
		Connections:   connections,
		Analysis:      analysis,
		Summary:       summary,
		FailedQueries: failedQueries,
	}, nil
}

// Reset clears internal state (no-op, state is per-request in context).
func (c *Collector) Reset() {}

// SetupContext implements collector.ContextSetup by initializing query tracking
// in the request context. This allows the profiler middleware to set up context
// once, so the same context is visible both to handlers and to CollectProfile.
func (c *Collector) SetupContext(ctx context.Context) context.Context {
	return WithContext(ctx)
}

// PanelMeta returns UI panel metadata for the GORM collector.
func (c *Collector) PanelMeta() collector.PanelMeta {
	return collector.PanelMeta{
		Name:      "gorm",
		Label:     "Database",
		Icon:      "database",
		Component: "GormPanel",
	}
}

// buildTransactionGroups organizes queries into transaction groups.
func buildTransactionGroups(queries []QueryEntry) []TransactionGroup {
	txMap := make(map[string]*TransactionGroup)
	txOrder := make([]string, 0)

	for _, q := range queries {
		if q.TransactionID == "" {
			continue
		}

		tx, exists := txMap[q.TransactionID]
		if !exists {
			tx = &TransactionGroup{
				ID:         q.TransactionID,
				Connection: q.Connection,
				Queries:    make([]QueryEntry, 0),
				Status:     "committed", // default, could be enhanced with explicit tracking
			}
			txMap[q.TransactionID] = tx
			txOrder = append(txOrder, q.TransactionID)
		}

		tx.Queries = append(tx.Queries, q)
		tx.TotalDurationMs += q.DurationMs

		// If any query in the transaction errored, mark as rolled back
		if q.Error != "" {
			tx.Status = "rolled_back"
		}
	}

	groups := make([]TransactionGroup, 0, len(txOrder))
	for _, id := range txOrder {
		groups = append(groups, *txMap[id])
	}
	return groups
}

// Middleware returns an HTTP middleware that initializes GORM query tracking
// in the request context. This must wrap your handlers for the collector to
// capture queries.
//
// Example:
//
//	mux.Handle("/", gormCollector.Middleware(yourHandler))
func (c *Collector) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithContext(r.Context())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
