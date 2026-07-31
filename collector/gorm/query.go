// Package gormcollector provides a GORM database query collector for the go-profiler.
// It captures SQL queries, parameters, execution time, and provides analysis features
// such as duplicate detection, N+1 query detection, and slow query highlighting.
package gormcollector

import "time"

// QueryEntry represents a single captured database query.
type QueryEntry struct {
	// SQL is the raw SQL query string.
	SQL string `json:"sql"`

	// Params contains the bound parameters for the query.
	Params []any `json:"params,omitempty"`

	// RunnableQuery is the SQL with parameters interpolated for copy-paste debugging.
	RunnableQuery string `json:"runnable_query"`

	// Duration is the query execution time in milliseconds.
	DurationMs float64 `json:"duration_ms"`

	// RowsAffected is the number of rows returned or affected.
	RowsAffected int64 `json:"rows_affected"`

	// Operation is the SQL operation type (SELECT, INSERT, UPDATE, DELETE, RAW).
	Operation string `json:"operation"`

	// Connection is the named connection that issued this query.
	Connection string `json:"connection"`

	// Error is the error message if the query failed, empty otherwise.
	Error string `json:"error,omitempty"`

	// TransactionID groups queries within the same transaction.
	// Empty if the query is not within an explicit transaction.
	TransactionID string `json:"transaction_id,omitempty"`

	// Backtrace is the Go call stack where the query originated.
	// Only populated when backtrace collection is enabled.
	Backtrace []string `json:"backtrace,omitempty"`

	// Timestamp is when the query was executed.
	Timestamp time.Time `json:"timestamp"`

	// Index is the sequential execution order within the request.
	Index int `json:"index"`
}

// TransactionGroup represents a group of queries executed within a single transaction.
type TransactionGroup struct {
	// ID is the unique identifier for this transaction.
	ID string `json:"id"`

	// Connection is the named connection this transaction belongs to.
	Connection string `json:"connection"`

	// Queries are the queries executed within this transaction.
	Queries []QueryEntry `json:"queries"`

	// TotalDurationMs is the total time spent on queries in this transaction.
	TotalDurationMs float64 `json:"total_duration_ms"`

	// Status is the transaction outcome: "committed", "rolled_back", or "pending".
	Status string `json:"status"`
}

// ConnectionData holds all query data for a single named database connection.
type ConnectionData struct {
	// Name is the connection identifier.
	Name string `json:"name"`

	// Queries are all queries executed on this connection (in order).
	Queries []QueryEntry `json:"queries"`

	// Transactions are the transaction groups on this connection.
	Transactions []TransactionGroup `json:"transactions"`

	// TotalDurationMs is the total time spent on all queries on this connection.
	TotalDurationMs float64 `json:"total_duration_ms"`

	// QueryCount is the total number of queries on this connection.
	QueryCount int `json:"query_count"`

	// FailedQueries are queries that resulted in an error.
	FailedQueries []QueryEntry `json:"failed_queries"`
}

// AnalysisResult holds the results of query analysis for a request.
type AnalysisResult struct {
	// DuplicateQueries are queries with identical SQL and parameters.
	DuplicateQueries []DuplicateGroup `json:"duplicate_queries,omitempty"`

	// SimilarQueries are queries with the same SQL but different parameters.
	SimilarQueries []SimilarGroup `json:"similar_queries,omitempty"`

	// N1Queries are detected N+1 query patterns.
	N1Queries []N1Group `json:"n1_queries,omitempty"`

	// SlowQueries are queries exceeding the configured threshold.
	SlowQueries []QueryEntry `json:"slow_queries,omitempty"`
}

// DuplicateGroup represents a set of identical queries (same SQL + same params).
type DuplicateGroup struct {
	// SQL is the common SQL string.
	SQL string `json:"sql"`

	// Params are the common parameters.
	Params []any `json:"params,omitempty"`

	// Count is how many times this exact query was executed.
	Count int `json:"count"`

	// Indices are the execution indices of the duplicate queries.
	Indices []int `json:"indices"`
}

// SimilarGroup represents queries with the same SQL structure but different parameters.
type SimilarGroup struct {
	// SQL is the common SQL template.
	SQL string `json:"sql"`

	// Count is how many times this SQL pattern was executed with different params.
	Count int `json:"count"`

	// Indices are the execution indices of the similar queries.
	Indices []int `json:"indices"`
}

// N1Group represents a detected N+1 query pattern.
type N1Group struct {
	// SQL is the repeated SQL pattern.
	SQL string `json:"sql"`

	// Count is the number of repeated executions.
	Count int `json:"count"`

	// Connection is the connection where the pattern was detected.
	Connection string `json:"connection"`

	// Indices are the execution indices of the N+1 queries.
	Indices []int `json:"indices"`
}

// Summary holds aggregate statistics for the request.
type Summary struct {
	// TotalQueries is the total number of queries executed.
	TotalQueries int `json:"total_queries"`

	// TotalDurationMs is the total time spent on all queries.
	TotalDurationMs float64 `json:"total_duration_ms"`

	// QueriesPerConnection maps connection name to query count.
	QueriesPerConnection map[string]int `json:"queries_per_connection"`

	// SlowestQuery is the query with the longest execution time.
	SlowestQuery *QueryEntry `json:"slowest_query,omitempty"`

	// DuplicateCount is the number of duplicate query groups found.
	DuplicateCount int `json:"duplicate_count"`

	// N1Count is the number of N+1 patterns detected.
	N1Count int `json:"n1_count"`

	// FailedCount is the total number of failed queries.
	FailedCount int `json:"failed_count"`

	// TransactionCount is the total number of transactions.
	TransactionCount int `json:"transaction_count"`
}

// GormData is the top-level data structure returned by the GORM collector.
// It is stored in Profile.CollectorData["gorm"].
type GormData struct {
	// Connections holds query data grouped by connection name.
	Connections []ConnectionData `json:"connections"`

	// Analysis contains the query analysis results.
	Analysis AnalysisResult `json:"analysis"`

	// Summary contains aggregate statistics.
	Summary Summary `json:"summary"`

	// FailedQueries contains all failed queries across all connections.
	FailedQueries []QueryEntry `json:"failed_queries"`
}
