package gormcollector

import (
	"testing"
	"time"
)

func TestDetectDuplicates(t *testing.T) {
	queries := []QueryEntry{
		{SQL: "SELECT * FROM users WHERE id = ?", Params: []any{1}, Index: 0},
		{SQL: "SELECT * FROM users WHERE id = ?", Params: []any{2}, Index: 1},
		{SQL: "SELECT * FROM users WHERE id = ?", Params: []any{1}, Index: 2}, // duplicate of index 0
		{SQL: "INSERT INTO logs (msg) VALUES (?)", Params: []any{"hello"}, Index: 3},
		{SQL: "INSERT INTO logs (msg) VALUES (?)", Params: []any{"hello"}, Index: 4}, // duplicate of index 3
	}

	result := detectDuplicates(queries)

	if len(result) != 2 {
		t.Fatalf("expected 2 duplicate groups, got %d", len(result))
	}

	// First group: SELECT with id=1
	if result[0].Count != 2 {
		t.Errorf("expected count 2 for first duplicate group, got %d", result[0].Count)
	}
	if result[0].SQL != "SELECT * FROM users WHERE id = ?" {
		t.Errorf("unexpected SQL in first group: %q", result[0].SQL)
	}

	// Second group: INSERT with "hello"
	if result[1].Count != 2 {
		t.Errorf("expected count 2 for second duplicate group, got %d", result[1].Count)
	}
}

func TestDetectDuplicatesNoDuplicates(t *testing.T) {
	queries := []QueryEntry{
		{SQL: "SELECT * FROM users WHERE id = ?", Params: []any{1}, Index: 0},
		{SQL: "SELECT * FROM users WHERE id = ?", Params: []any{2}, Index: 1},
		{SQL: "SELECT * FROM posts WHERE id = ?", Params: []any{1}, Index: 2},
	}

	result := detectDuplicates(queries)
	if len(result) != 0 {
		t.Errorf("expected no duplicate groups, got %d", len(result))
	}
}

func TestDetectSimilar(t *testing.T) {
	queries := []QueryEntry{
		{SQL: "SELECT * FROM users WHERE id = ?", Params: []any{1}, Index: 0},
		{SQL: "SELECT * FROM users WHERE id = ?", Params: []any{2}, Index: 1},
		{SQL: "SELECT * FROM users WHERE id = ?", Params: []any{3}, Index: 2},
		{SQL: "INSERT INTO logs (msg) VALUES (?)", Params: []any{"hello"}, Index: 3},
	}

	result := detectSimilar(queries)

	if len(result) != 1 {
		t.Fatalf("expected 1 similar group, got %d", len(result))
	}

	if result[0].Count != 3 {
		t.Errorf("expected count 3, got %d", result[0].Count)
	}
}

func TestDetectSimilarWithWhitespace(t *testing.T) {
	queries := []QueryEntry{
		{SQL: "SELECT * FROM users WHERE id = ?", Index: 0},
		{SQL: "SELECT *  FROM  users  WHERE  id = ?", Index: 1}, // extra whitespace
	}

	result := detectSimilar(queries)

	if len(result) != 1 {
		t.Fatalf("expected 1 similar group (whitespace normalized), got %d", len(result))
	}
	if result[0].Count != 2 {
		t.Errorf("expected count 2, got %d", result[0].Count)
	}
}

func TestDetectN1(t *testing.T) {
	// Simulate N+1: 1 query to get list, then N queries to get related data
	queries := []QueryEntry{
		{SQL: "SELECT * FROM users", Connection: "main", Index: 0},
		{SQL: "SELECT * FROM profiles WHERE user_id = ?", Params: []any{1}, Connection: "main", Index: 1},
		{SQL: "SELECT * FROM profiles WHERE user_id = ?", Params: []any{2}, Connection: "main", Index: 2},
		{SQL: "SELECT * FROM profiles WHERE user_id = ?", Params: []any{3}, Connection: "main", Index: 3},
		{SQL: "SELECT * FROM profiles WHERE user_id = ?", Params: []any{4}, Connection: "main", Index: 4},
		{SQL: "SELECT * FROM profiles WHERE user_id = ?", Params: []any{5}, Connection: "main", Index: 5},
	}

	// Threshold of 5
	result := detectN1(queries, 5)

	if len(result) != 1 {
		t.Fatalf("expected 1 N+1 group, got %d", len(result))
	}

	if result[0].Count != 5 {
		t.Errorf("expected N+1 count 5, got %d", result[0].Count)
	}
	if result[0].Connection != "main" {
		t.Errorf("expected connection 'main', got %q", result[0].Connection)
	}
}

func TestDetectN1BelowThreshold(t *testing.T) {
	queries := []QueryEntry{
		{SQL: "SELECT * FROM profiles WHERE user_id = ?", Params: []any{1}, Connection: "main", Index: 0},
		{SQL: "SELECT * FROM profiles WHERE user_id = ?", Params: []any{2}, Connection: "main", Index: 1},
		{SQL: "SELECT * FROM profiles WHERE user_id = ?", Params: []any{3}, Connection: "main", Index: 2},
	}

	// Threshold of 5 — only 3 queries, should not trigger
	result := detectN1(queries, 5)

	if len(result) != 0 {
		t.Errorf("expected no N+1 detection below threshold, got %d", len(result))
	}
}

func TestDetectSlow(t *testing.T) {
	opts := defaultOptions()
	opts.SlowThreshold = 100 * time.Millisecond

	queries := []QueryEntry{
		{SQL: "SELECT 1", DurationMs: 5.0, Connection: "main", Index: 0},
		{SQL: "SELECT * FROM large_table", DurationMs: 150.0, Connection: "main", Index: 1},
		{SQL: "UPDATE counters SET n = n + 1", DurationMs: 50.0, Connection: "main", Index: 2},
		{SQL: "SELECT * FROM another_table JOIN ...", DurationMs: 250.0, Connection: "main", Index: 3},
	}

	result := detectSlow(queries, opts)

	if len(result) != 2 {
		t.Fatalf("expected 2 slow queries, got %d", len(result))
	}

	if result[0].DurationMs != 150.0 {
		t.Errorf("expected first slow query duration 150ms, got %.1f", result[0].DurationMs)
	}
	if result[1].DurationMs != 250.0 {
		t.Errorf("expected second slow query duration 250ms, got %.1f", result[1].DurationMs)
	}
}

func TestDetectSlowPerConnection(t *testing.T) {
	opts := defaultOptions()
	opts.SlowThreshold = 100 * time.Millisecond
	opts.Connections = []ConnectionConfig{
		{Name: "fast-db", SlowThreshold: 50 * time.Millisecond},
		{Name: "slow-db", SlowThreshold: 200 * time.Millisecond},
	}

	queries := []QueryEntry{
		{SQL: "SELECT 1", DurationMs: 75.0, Connection: "fast-db", Index: 0},   // slow for fast-db (>50ms)
		{SQL: "SELECT 1", DurationMs: 75.0, Connection: "slow-db", Index: 1},   // NOT slow for slow-db (<200ms)
		{SQL: "SELECT 1", DurationMs: 250.0, Connection: "slow-db", Index: 2},  // slow for slow-db (>200ms)
	}

	result := detectSlow(queries, opts)

	if len(result) != 2 {
		t.Fatalf("expected 2 slow queries (per-connection thresholds), got %d", len(result))
	}

	if result[0].Connection != "fast-db" {
		t.Errorf("expected first slow query on fast-db, got %q", result[0].Connection)
	}
	if result[1].Connection != "slow-db" {
		t.Errorf("expected second slow query on slow-db, got %q", result[1].Connection)
	}
}

func TestBuildSummary(t *testing.T) {
	connections := []ConnectionData{
		{
			Name:            "main",
			QueryCount:      5,
			TotalDurationMs: 100.0,
			Queries: []QueryEntry{
				{DurationMs: 50.0, Index: 0},
				{DurationMs: 10.0, Index: 1},
				{DurationMs: 15.0, Index: 2},
				{DurationMs: 5.0, Index: 3},
				{DurationMs: 20.0, Index: 4},
			},
			FailedQueries: []QueryEntry{{Index: 2}},
			Transactions:  []TransactionGroup{{ID: "tx1"}},
		},
		{
			Name:            "secondary",
			QueryCount:      2,
			TotalDurationMs: 30.0,
			Queries: []QueryEntry{
				{DurationMs: 20.0, Index: 5},
				{DurationMs: 10.0, Index: 6},
			},
			FailedQueries: []QueryEntry{},
			Transactions:  []TransactionGroup{},
		},
	}

	analysis := AnalysisResult{
		DuplicateQueries: []DuplicateGroup{{Count: 3}},
		N1Queries:        []N1Group{{Count: 5}, {Count: 6}},
	}

	summary := buildSummary(connections, analysis)

	if summary.TotalQueries != 7 {
		t.Errorf("expected 7 total queries, got %d", summary.TotalQueries)
	}
	if summary.TotalDurationMs != 130.0 {
		t.Errorf("expected 130ms total duration, got %.1f", summary.TotalDurationMs)
	}
	if summary.QueriesPerConnection["main"] != 5 {
		t.Errorf("expected 5 queries for main, got %d", summary.QueriesPerConnection["main"])
	}
	if summary.QueriesPerConnection["secondary"] != 2 {
		t.Errorf("expected 2 queries for secondary, got %d", summary.QueriesPerConnection["secondary"])
	}
	if summary.SlowestQuery == nil || summary.SlowestQuery.DurationMs != 50.0 {
		t.Errorf("expected slowest query 50ms")
	}
	if summary.DuplicateCount != 1 {
		t.Errorf("expected 1 duplicate group, got %d", summary.DuplicateCount)
	}
	if summary.N1Count != 2 {
		t.Errorf("expected 2 N+1 groups, got %d", summary.N1Count)
	}
	if summary.FailedCount != 1 {
		t.Errorf("expected 1 failed query, got %d", summary.FailedCount)
	}
	if summary.TransactionCount != 1 {
		t.Errorf("expected 1 transaction, got %d", summary.TransactionCount)
	}
}

func TestNormalizeSQL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SELECT * FROM users", "select * from users"},
		{"  SELECT *  FROM  users  ", "select * from users"},
		{"SELECT\n*\nFROM\nusers", "select * from users"},
		{"select * from users where id = ?", "select * from users where id = ?"},
	}

	for _, tt := range tests {
		result := normalizeSQL(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeSQL(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCollapseWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello  world", "hello world"},
		{"a\tb\nc", "a b c"},
		{"  leading", " leading"},
		{"trailing  ", "trailing "},
		{"no-change", "no-change"},
	}

	for _, tt := range tests {
		result := collapseWhitespace(tt.input)
		if result != tt.expected {
			t.Errorf("collapseWhitespace(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestHashQuery(t *testing.T) {
	// Same SQL + same params = same hash
	h1 := hashQuery("SELECT * FROM users WHERE id = ?", []any{1})
	h2 := hashQuery("SELECT * FROM users WHERE id = ?", []any{1})
	if h1 != h2 {
		t.Error("expected same hash for identical queries")
	}

	// Same SQL + different params = different hash
	h3 := hashQuery("SELECT * FROM users WHERE id = ?", []any{2})
	if h1 == h3 {
		t.Error("expected different hash for different params")
	}

	// Different SQL = different hash
	h4 := hashQuery("SELECT * FROM posts WHERE id = ?", []any{1})
	if h1 == h4 {
		t.Error("expected different hash for different SQL")
	}
}
