package gormcollector

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// analyze runs all analysis checks on the given queries and returns the results.
func analyze(queries []QueryEntry, opts *Options) AnalysisResult {
	return AnalysisResult{
		DuplicateQueries: detectDuplicates(queries),
		SimilarQueries:   detectSimilar(queries),
		N1Queries:        detectN1(queries, opts.N1Threshold),
		SlowQueries:      detectSlow(queries, opts),
	}
}

// detectDuplicates finds queries with identical SQL and parameters.
func detectDuplicates(queries []QueryEntry) []DuplicateGroup {
	type key struct {
		hash string
	}

	groups := make(map[string]*DuplicateGroup)
	order := make([]string, 0)

	for _, q := range queries {
		h := hashQuery(q.SQL, q.Params)
		if g, exists := groups[h]; exists {
			g.Count++
			g.Indices = append(g.Indices, q.Index)
		} else {
			groups[h] = &DuplicateGroup{
				SQL:     q.SQL,
				Params:  q.Params,
				Count:   1,
				Indices: []int{q.Index},
			}
			order = append(order, h)
		}
	}

	// Only return groups with count > 1
	var result []DuplicateGroup
	for _, h := range order {
		if groups[h].Count > 1 {
			result = append(result, *groups[h])
		}
	}
	return result
}

// detectSimilar finds queries with the same SQL template but different parameters.
func detectSimilar(queries []QueryEntry) []SimilarGroup {
	groups := make(map[string]*SimilarGroup)
	order := make([]string, 0)

	for _, q := range queries {
		sql := normalizeSQL(q.SQL)
		if g, exists := groups[sql]; exists {
			g.Count++
			g.Indices = append(g.Indices, q.Index)
		} else {
			groups[sql] = &SimilarGroup{
				SQL:     q.SQL,
				Count:   1,
				Indices: []int{q.Index},
			}
			order = append(order, sql)
		}
	}

	// Only return groups with count > 1
	var result []SimilarGroup
	for _, sql := range order {
		if groups[sql].Count > 1 {
			result = append(result, *groups[sql])
		}
	}
	return result
}

// detectN1 identifies N+1 query patterns — the same SQL pattern repeated
// many times with different parameter values.
func detectN1(queries []QueryEntry, threshold int) []N1Group {
	if threshold <= 0 {
		threshold = DefaultN1Threshold
	}

	// Group consecutive similar queries by connection
	type seqKey struct {
		sql        string
		connection string
	}

	groups := make(map[seqKey]*N1Group)
	order := make([]seqKey, 0)

	for _, q := range queries {
		k := seqKey{sql: normalizeSQL(q.SQL), connection: q.Connection}
		if g, exists := groups[k]; exists {
			g.Count++
			g.Indices = append(g.Indices, q.Index)
		} else {
			groups[k] = &N1Group{
				SQL:        q.SQL,
				Count:      1,
				Connection: q.Connection,
				Indices:    []int{q.Index},
			}
			order = append(order, k)
		}
	}

	// Only return groups exceeding the threshold
	var result []N1Group
	for _, k := range order {
		if groups[k].Count >= threshold {
			result = append(result, *groups[k])
		}
	}
	return result
}

// detectSlow identifies queries that exceed the configured slow threshold.
func detectSlow(queries []QueryEntry, opts *Options) []QueryEntry {
	var result []QueryEntry
	for _, q := range queries {
		threshold := opts.slowThresholdFor(q.Connection)
		durationThresholdMs := float64(threshold) / float64(time.Millisecond)
		if q.DurationMs >= durationThresholdMs {
			result = append(result, q)
		}
	}
	return result
}

// buildSummary computes aggregate statistics from the collected data.
func buildSummary(connections []ConnectionData, analysis AnalysisResult) Summary {
	s := Summary{
		QueriesPerConnection: make(map[string]int),
	}

	for _, conn := range connections {
		s.TotalQueries += conn.QueryCount
		s.TotalDurationMs += conn.TotalDurationMs
		s.QueriesPerConnection[conn.Name] = conn.QueryCount
		s.FailedCount += len(conn.FailedQueries)
		s.TransactionCount += len(conn.Transactions)

		for i := range conn.Queries {
			q := &conn.Queries[i]
			if s.SlowestQuery == nil || q.DurationMs > s.SlowestQuery.DurationMs {
				s.SlowestQuery = q
			}
		}
	}

	s.DuplicateCount = len(analysis.DuplicateQueries)
	s.N1Count = len(analysis.N1Queries)

	return s
}

// hashQuery creates a hash from SQL and parameters for duplicate detection.
func hashQuery(sql string, params []any) string {
	h := sha256.New()
	h.Write([]byte(sql))
	for _, p := range params {
		h.Write([]byte(fmt.Sprintf("|%v", p)))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// normalizeSQL strips parameter values to create a comparable SQL template.
// This replaces quoted strings and numbers with placeholders.
func normalizeSQL(sql string) string {
	// Simple normalization: collapse whitespace and lowercase
	normalized := strings.ToLower(strings.TrimSpace(sql))
	normalized = collapseWhitespace(normalized)
	return normalized
}

// collapseWhitespace replaces multiple whitespace characters with a single space.
func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
		} else {
			b.WriteRune(r)
			inSpace = false
		}
	}
	return b.String()
}
