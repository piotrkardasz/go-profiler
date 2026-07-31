package gormcollector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/piotrkardasz/go-profiler/collector"
)

func TestCollectorName(t *testing.T) {
	c := &Collector{opts: defaultOptions()}
	if c.Name() != "gorm" {
		t.Errorf("expected collector name 'gorm', got %q", c.Name())
	}
}

func TestCollectorPanelMeta(t *testing.T) {
	c := &Collector{opts: defaultOptions()}
	meta := c.PanelMeta()

	if meta.Name != "gorm" {
		t.Errorf("expected panel name 'gorm', got %q", meta.Name)
	}
	if meta.Label != "Database" {
		t.Errorf("expected panel label 'Database', got %q", meta.Label)
	}
	if meta.Icon != "database" {
		t.Errorf("expected panel icon 'database', got %q", meta.Icon)
	}
	if meta.Component != "GormPanel" {
		t.Errorf("expected panel component 'GormPanel', got %q", meta.Component)
	}
}

func TestCollectorImplementsInterfaces(t *testing.T) {
	c := &Collector{opts: defaultOptions()}

	// Verify it implements collector.Collector
	var _ collector.Collector = c

	// Verify it implements collector.PanelProvider
	var _ collector.PanelProvider = c
}

func TestCollectWithNoContext(t *testing.T) {
	c := &Collector{opts: defaultOptions()}
	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := collector.ResponseData{StatusCode: 200}

	data, err := c.Collect(ctx, req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gormData, ok := data.(*GormData)
	if !ok {
		t.Fatal("expected *GormData")
	}

	if len(gormData.Connections) != 0 {
		t.Errorf("expected 0 connections, got %d", len(gormData.Connections))
	}
	if gormData.Summary.TotalQueries != 0 {
		t.Errorf("expected 0 total queries, got %d", gormData.Summary.TotalQueries)
	}
}

func TestCollectWithQueries(t *testing.T) {
	c := &Collector{opts: defaultOptions()}

	// Simulate a request context with queries
	ctx := WithContext(context.Background())

	// Manually inject queries (simulating what the plugin would do)
	rq := queriesFromContext(ctx)
	rq.queries = []QueryEntry{
		{
			SQL:           "SELECT * FROM users WHERE id = ?",
			Params:        []any{1},
			RunnableQuery: "SELECT * FROM users WHERE id = 1",
			DurationMs:    5.2,
			RowsAffected:  1,
			Operation:     "SELECT",
			Connection:    "postgres-main",
			Timestamp:     time.Now(),
			Index:         0,
		},
		{
			SQL:           "INSERT INTO logs (message) VALUES (?)",
			Params:        []any{"user login"},
			RunnableQuery: "INSERT INTO logs (message) VALUES ('user login')",
			DurationMs:    2.1,
			RowsAffected:  1,
			Operation:     "INSERT",
			Connection:    "postgres-main",
			Timestamp:     time.Now(),
			Index:         1,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := collector.ResponseData{StatusCode: 200}

	data, err := c.Collect(ctx, req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gormData := data.(*GormData)

	if len(gormData.Connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(gormData.Connections))
	}

	conn := gormData.Connections[0]
	if conn.Name != "postgres-main" {
		t.Errorf("expected connection name 'postgres-main', got %q", conn.Name)
	}
	if conn.QueryCount != 2 {
		t.Errorf("expected 2 queries, got %d", conn.QueryCount)
	}
	expectedDuration := 7.3
	if conn.TotalDurationMs < expectedDuration-0.01 || conn.TotalDurationMs > expectedDuration+0.01 {
		t.Errorf("expected total duration ~7.3ms, got %.3fms", conn.TotalDurationMs)
	}

	// Summary
	if gormData.Summary.TotalQueries != 2 {
		t.Errorf("expected total queries 2, got %d", gormData.Summary.TotalQueries)
	}
	if gormData.Summary.QueriesPerConnection["postgres-main"] != 2 {
		t.Errorf("expected 2 queries for postgres-main")
	}
}

func TestCollectMultipleConnections(t *testing.T) {
	c := &Collector{opts: defaultOptions()}

	ctx := WithContext(context.Background())
	rq := queriesFromContext(ctx)
	rq.queries = []QueryEntry{
		{
			SQL:        "SELECT * FROM users",
			DurationMs: 3.0,
			Operation:  "SELECT",
			Connection: "postgres-main",
			Index:      0,
		},
		{
			SQL:        "SELECT * FROM analytics",
			DurationMs: 10.0,
			Operation:  "SELECT",
			Connection: "mysql-analytics",
			Index:      1,
		},
		{
			SQL:        "UPDATE users SET name = ?",
			DurationMs: 2.0,
			Operation:  "UPDATE",
			Connection: "postgres-main",
			Index:      2,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := collector.ResponseData{StatusCode: 200}

	data, err := c.Collect(ctx, req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gormData := data.(*GormData)

	if len(gormData.Connections) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(gormData.Connections))
	}

	// Verify connection order (first seen)
	if gormData.Connections[0].Name != "postgres-main" {
		t.Errorf("expected first connection 'postgres-main', got %q", gormData.Connections[0].Name)
	}
	if gormData.Connections[1].Name != "mysql-analytics" {
		t.Errorf("expected second connection 'mysql-analytics', got %q", gormData.Connections[1].Name)
	}

	// Postgres: 2 queries, MySQL: 1 query
	if gormData.Connections[0].QueryCount != 2 {
		t.Errorf("expected 2 queries for postgres, got %d", gormData.Connections[0].QueryCount)
	}
	if gormData.Connections[1].QueryCount != 1 {
		t.Errorf("expected 1 query for mysql, got %d", gormData.Connections[1].QueryCount)
	}

	// Summary
	if gormData.Summary.TotalQueries != 3 {
		t.Errorf("expected 3 total queries, got %d", gormData.Summary.TotalQueries)
	}
}

func TestCollectFailedQueries(t *testing.T) {
	c := &Collector{opts: defaultOptions()}

	ctx := WithContext(context.Background())
	rq := queriesFromContext(ctx)
	rq.queries = []QueryEntry{
		{
			SQL:        "SELECT * FROM users",
			DurationMs: 3.0,
			Operation:  "SELECT",
			Connection: "main",
			Index:      0,
		},
		{
			SQL:        "INSERT INTO users (email) VALUES (?)",
			DurationMs: 1.5,
			Operation:  "INSERT",
			Connection: "main",
			Error:      "duplicate key value violates unique constraint",
			Index:      1,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := collector.ResponseData{StatusCode: 200}

	data, err := c.Collect(ctx, req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gormData := data.(*GormData)

	if len(gormData.FailedQueries) != 1 {
		t.Fatalf("expected 1 failed query, got %d", len(gormData.FailedQueries))
	}
	if gormData.FailedQueries[0].Error != "duplicate key value violates unique constraint" {
		t.Errorf("unexpected error message: %q", gormData.FailedQueries[0].Error)
	}
	if gormData.Summary.FailedCount != 1 {
		t.Errorf("expected failed count 1, got %d", gormData.Summary.FailedCount)
	}
}

func TestCollectTransactionGroups(t *testing.T) {
	c := &Collector{opts: defaultOptions()}

	ctx := WithContext(context.Background())
	rq := queriesFromContext(ctx)
	rq.queries = []QueryEntry{
		{
			SQL:           "SELECT * FROM users",
			DurationMs:    1.0,
			Operation:     "SELECT",
			Connection:    "main",
			TransactionID: "",
			Index:         0,
		},
		{
			SQL:           "INSERT INTO orders (user_id) VALUES (?)",
			DurationMs:    2.0,
			Operation:     "INSERT",
			Connection:    "main",
			TransactionID: "tx-abc123",
			Index:         1,
		},
		{
			SQL:           "UPDATE inventory SET stock = stock - 1 WHERE id = ?",
			DurationMs:    3.0,
			Operation:     "UPDATE",
			Connection:    "main",
			TransactionID: "tx-abc123",
			Index:         2,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := collector.ResponseData{StatusCode: 200}

	data, err := c.Collect(ctx, req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gormData := data.(*GormData)

	conn := gormData.Connections[0]
	if len(conn.Transactions) != 1 {
		t.Fatalf("expected 1 transaction group, got %d", len(conn.Transactions))
	}

	tx := conn.Transactions[0]
	if tx.ID != "tx-abc123" {
		t.Errorf("expected transaction ID 'tx-abc123', got %q", tx.ID)
	}
	if len(tx.Queries) != 2 {
		t.Errorf("expected 2 queries in transaction, got %d", len(tx.Queries))
	}
	if tx.TotalDurationMs != 5.0 {
		t.Errorf("expected tx duration 5.0ms, got %.1fms", tx.TotalDurationMs)
	}
	if tx.Status != "committed" {
		t.Errorf("expected tx status 'committed', got %q", tx.Status)
	}

	if gormData.Summary.TransactionCount != 1 {
		t.Errorf("expected 1 transaction in summary, got %d", gormData.Summary.TransactionCount)
	}
}

func TestCollectTransactionRolledBack(t *testing.T) {
	c := &Collector{opts: defaultOptions()}

	ctx := WithContext(context.Background())
	rq := queriesFromContext(ctx)
	rq.queries = []QueryEntry{
		{
			SQL:           "INSERT INTO orders (user_id) VALUES (?)",
			DurationMs:    2.0,
			Operation:     "INSERT",
			Connection:    "main",
			TransactionID: "tx-fail",
			Index:         0,
		},
		{
			SQL:           "UPDATE inventory SET stock = stock - 1 WHERE id = ?",
			DurationMs:    3.0,
			Operation:     "UPDATE",
			Connection:    "main",
			TransactionID: "tx-fail",
			Error:         "check constraint violated",
			Index:         1,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := collector.ResponseData{StatusCode: 200}

	data, err := c.Collect(ctx, req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gormData := data.(*GormData)
	tx := gormData.Connections[0].Transactions[0]
	if tx.Status != "rolled_back" {
		t.Errorf("expected tx status 'rolled_back', got %q", tx.Status)
	}
}

func TestMiddleware(t *testing.T) {
	c := &Collector{opts: defaultOptions()}

	// Create a handler that checks context has query tracking
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rq := queriesFromContext(r.Context())
		if rq == nil {
			t.Error("expected query tracking in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with the collector middleware
	wrapped := c.Middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestWithContext(t *testing.T) {
	ctx := context.Background()

	// Before: no query tracking
	if queriesFromContext(ctx) != nil {
		t.Error("expected no query tracking in background context")
	}

	// After: query tracking initialized
	ctx = WithContext(ctx)
	rq := queriesFromContext(ctx)
	if rq == nil {
		t.Fatal("expected query tracking in context")
	}
	if len(rq.queries) != 0 {
		t.Errorf("expected 0 queries initially, got %d", len(rq.queries))
	}
}

func TestWithTransaction(t *testing.T) {
	ctx := context.Background()

	// No transaction by default
	if _, ok := TransactionIDFromContext(ctx); ok {
		t.Error("expected no transaction ID in background context")
	}

	// After WithTransaction
	ctx = WithTransaction(ctx)
	txID, ok := TransactionIDFromContext(ctx)
	if !ok {
		t.Fatal("expected transaction ID in context")
	}
	if txID == "" {
		t.Error("expected non-empty transaction ID")
	}
	if len(txID) < 5 {
		t.Errorf("transaction ID too short: %q", txID)
	}
}
