package http

import (
	"crypto/sha256"
	"fmt"
)

// analyze runs all analysis passes on the captured HTTP calls.
func analyze(calls []HTTPCallEntry, opts *options) AnalysisResult {
	result := AnalysisResult{}

	if len(calls) == 0 {
		return result
	}

	result.SlowCalls = detectSlow(calls, opts)
	result.FailedCalls = detectFailed(calls)

	if opts.duplicateDetection {
		result.DuplicateCalls = detectDuplicates(calls)
	}

	return result
}

// detectSlow returns calls exceeding the slow threshold.
func detectSlow(calls []HTTPCallEntry, opts *options) []HTTPCallEntry {
	thresholdMs := float64(opts.slowThreshold.Milliseconds())
	var slow []HTTPCallEntry

	for _, c := range calls {
		if c.DurationMs >= thresholdMs {
			slow = append(slow, c)
		}
	}

	return slow
}

// detectFailed returns calls with non-2xx status codes or transport errors.
func detectFailed(calls []HTTPCallEntry) []HTTPCallEntry {
	var failed []HTTPCallEntry

	for _, c := range calls {
		if c.Error != "" {
			failed = append(failed, c)
			continue
		}
		if c.StatusCode > 0 && (c.StatusCode < 200 || c.StatusCode >= 300) {
			failed = append(failed, c)
		}
	}

	return failed
}

// detectDuplicates finds repeated identical requests (same method + URL + body hash).
func detectDuplicates(calls []HTTPCallEntry) []DuplicateGroup {
	type group struct {
		method  string
		url     string
		indices []int
	}

	groups := make(map[string]*group)
	order := make([]string, 0)

	for _, c := range calls {
		key := duplicateKey(c.Method, c.URL, c.RequestBody)
		if g, ok := groups[key]; ok {
			g.indices = append(g.indices, c.Index)
		} else {
			groups[key] = &group{
				method:  c.Method,
				url:     c.URL,
				indices: []int{c.Index},
			}
			order = append(order, key)
		}
	}

	var duplicates []DuplicateGroup
	for _, key := range order {
		g := groups[key]
		if len(g.indices) > 1 {
			duplicates = append(duplicates, DuplicateGroup{
				Method:  g.method,
				URL:     g.url,
				Count:   len(g.indices),
				Indices: g.indices,
			})
		}
	}

	return duplicates
}

// duplicateKey generates a hash key for duplicate detection.
func duplicateKey(method, url, body string) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte("\x00"))
	h.Write([]byte(url))
	h.Write([]byte("\x00"))
	h.Write([]byte(body))
	return fmt.Sprintf("%x", h.Sum(nil)[:16])
}

// buildSummary computes aggregate statistics from calls and analysis results.
func buildSummary(calls []HTTPCallEntry, analysis AnalysisResult) Summary {
	summary := Summary{
		TotalCalls:      len(calls),
		CallsPerService: make(map[string]int),
	}

	if len(calls) == 0 {
		return summary
	}

	var slowest *HTTPCallEntry
	for i := range calls {
		c := &calls[i]
		summary.TotalDurationMs += c.DurationMs
		summary.CallsPerService[c.Service]++

		if slowest == nil || c.DurationMs > slowest.DurationMs {
			slowest = c
		}
	}

	if slowest != nil {
		entry := *slowest
		summary.SlowestCall = &entry
	}

	summary.FailedCount = len(analysis.FailedCalls)
	summary.SlowCount = len(analysis.SlowCalls)

	for _, dg := range analysis.DuplicateCalls {
		summary.DuplicateCount += dg.Count - 1 // count extra calls beyond the first
	}

	return summary
}
