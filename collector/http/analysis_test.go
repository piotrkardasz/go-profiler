package http

import (
	"testing"
	"time"
)

func TestDetectSlow_NoSlowCalls(t *testing.T) {
	calls := []HTTPCallEntry{
		{Index: 0, DurationMs: 10},
		{Index: 1, DurationMs: 50},
		{Index: 2, DurationMs: 100},
	}
	opts := &options{slowThreshold: 500 * time.Millisecond}

	slow := detectSlow(calls, opts)
	if len(slow) != 0 {
		t.Errorf("expected no slow calls, got %d", len(slow))
	}
}

func TestDetectSlow_WithSlowCalls(t *testing.T) {
	calls := []HTTPCallEntry{
		{Index: 0, DurationMs: 10},
		{Index: 1, DurationMs: 600},
		{Index: 2, DurationMs: 500},
		{Index: 3, DurationMs: 1200},
	}
	opts := &options{slowThreshold: 500 * time.Millisecond}

	slow := detectSlow(calls, opts)
	if len(slow) != 3 {
		t.Errorf("expected 3 slow calls (500, 600, 1200ms), got %d", len(slow))
	}
}

func TestDetectSlow_CustomThreshold(t *testing.T) {
	calls := []HTTPCallEntry{
		{Index: 0, DurationMs: 50},
		{Index: 1, DurationMs: 150},
	}
	opts := &options{slowThreshold: 100 * time.Millisecond}

	slow := detectSlow(calls, opts)
	if len(slow) != 1 {
		t.Errorf("expected 1 slow call, got %d", len(slow))
	}
	if slow[0].Index != 1 {
		t.Errorf("expected slow call index 1, got %d", slow[0].Index)
	}
}

func TestDetectFailed_Non2xxStatus(t *testing.T) {
	calls := []HTTPCallEntry{
		{Index: 0, StatusCode: 200},
		{Index: 1, StatusCode: 404},
		{Index: 2, StatusCode: 500},
		{Index: 3, StatusCode: 301},
		{Index: 4, StatusCode: 201},
	}

	failed := detectFailed(calls)
	if len(failed) != 3 {
		t.Errorf("expected 3 failed calls (404, 500, 301), got %d", len(failed))
	}
}

func TestDetectFailed_TransportErrors(t *testing.T) {
	calls := []HTTPCallEntry{
		{Index: 0, StatusCode: 0, Error: "connection refused"},
		{Index: 1, StatusCode: 200},
		{Index: 2, StatusCode: 0, Error: "timeout"},
	}

	failed := detectFailed(calls)
	if len(failed) != 2 {
		t.Errorf("expected 2 failed calls, got %d", len(failed))
	}
}

func TestDetectFailed_EmptyCalls(t *testing.T) {
	failed := detectFailed(nil)
	if failed != nil {
		t.Errorf("expected nil for empty calls, got %v", failed)
	}
}

func TestDetectDuplicates_NoDuplicates(t *testing.T) {
	calls := []HTTPCallEntry{
		{Index: 0, Method: "GET", URL: "http://a/1"},
		{Index: 1, Method: "GET", URL: "http://a/2"},
		{Index: 2, Method: "POST", URL: "http://a/1"},
	}

	dups := detectDuplicates(calls)
	if len(dups) != 0 {
		t.Errorf("expected no duplicates, got %d", len(dups))
	}
}

func TestDetectDuplicates_WithDuplicates(t *testing.T) {
	calls := []HTTPCallEntry{
		{Index: 0, Method: "GET", URL: "http://a/1"},
		{Index: 1, Method: "GET", URL: "http://a/2"},
		{Index: 2, Method: "GET", URL: "http://a/1"},
		{Index: 3, Method: "GET", URL: "http://a/1"},
	}

	dups := detectDuplicates(calls)
	if len(dups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(dups))
	}

	g := dups[0]
	if g.Method != "GET" {
		t.Errorf("expected method GET, got %q", g.Method)
	}
	if g.URL != "http://a/1" {
		t.Errorf("expected URL 'http://a/1', got %q", g.URL)
	}
	if g.Count != 3 {
		t.Errorf("expected count 3, got %d", g.Count)
	}
	if len(g.Indices) != 3 || g.Indices[0] != 0 || g.Indices[1] != 2 || g.Indices[2] != 3 {
		t.Errorf("expected indices [0,2,3], got %v", g.Indices)
	}
}

func TestDetectDuplicates_DifferentBodies(t *testing.T) {
	calls := []HTTPCallEntry{
		{Index: 0, Method: "POST", URL: "http://a/1", RequestBody: `{"id":1}`},
		{Index: 1, Method: "POST", URL: "http://a/1", RequestBody: `{"id":2}`},
	}

	dups := detectDuplicates(calls)
	if len(dups) != 0 {
		t.Error("expected no duplicates for different bodies")
	}
}

func TestDetectDuplicates_SameBodies(t *testing.T) {
	calls := []HTTPCallEntry{
		{Index: 0, Method: "POST", URL: "http://a/1", RequestBody: `{"id":1}`},
		{Index: 1, Method: "POST", URL: "http://a/1", RequestBody: `{"id":1}`},
	}

	dups := detectDuplicates(calls)
	if len(dups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(dups))
	}
	if dups[0].Count != 2 {
		t.Errorf("expected count 2, got %d", dups[0].Count)
	}
}

func TestBuildSummary_EmptyCalls(t *testing.T) {
	summary := buildSummary(nil, AnalysisResult{})
	if summary.TotalCalls != 0 {
		t.Errorf("expected 0 total calls, got %d", summary.TotalCalls)
	}
	if summary.SlowestCall != nil {
		t.Error("expected nil slowest call")
	}
}

func TestBuildSummary_Complete(t *testing.T) {
	calls := []HTTPCallEntry{
		{Index: 0, Service: "svc-a", DurationMs: 10},
		{Index: 1, Service: "svc-a", DurationMs: 100},
		{Index: 2, Service: "svc-b", DurationMs: 50},
		{Index: 3, Service: "svc-b", DurationMs: 200, StatusCode: 500},
	}

	analysis := AnalysisResult{
		SlowCalls:   []HTTPCallEntry{calls[3]},
		FailedCalls: []HTTPCallEntry{calls[3]},
		DuplicateCalls: []DuplicateGroup{
			{Method: "GET", URL: "http://a", Count: 2, Indices: []int{0, 1}},
		},
	}

	summary := buildSummary(calls, analysis)

	if summary.TotalCalls != 4 {
		t.Errorf("expected 4 total calls, got %d", summary.TotalCalls)
	}
	if summary.TotalDurationMs != 360 {
		t.Errorf("expected total duration 360ms, got %f", summary.TotalDurationMs)
	}
	if summary.CallsPerService["svc-a"] != 2 {
		t.Errorf("expected 2 calls for svc-a, got %d", summary.CallsPerService["svc-a"])
	}
	if summary.CallsPerService["svc-b"] != 2 {
		t.Errorf("expected 2 calls for svc-b, got %d", summary.CallsPerService["svc-b"])
	}
	if summary.FailedCount != 1 {
		t.Errorf("expected 1 failed, got %d", summary.FailedCount)
	}
	if summary.SlowCount != 1 {
		t.Errorf("expected 1 slow, got %d", summary.SlowCount)
	}
	if summary.DuplicateCount != 1 {
		t.Errorf("expected 1 duplicate, got %d", summary.DuplicateCount)
	}
	if summary.SlowestCall == nil {
		t.Fatal("expected slowest call")
	}
	if summary.SlowestCall.DurationMs != 200 {
		t.Errorf("expected slowest call duration 200ms, got %f", summary.SlowestCall.DurationMs)
	}
}

func TestAnalyze_DuplicateDetectionDisabled(t *testing.T) {
	calls := []HTTPCallEntry{
		{Index: 0, Method: "GET", URL: "http://a/1", DurationMs: 10, StatusCode: 200},
		{Index: 1, Method: "GET", URL: "http://a/1", DurationMs: 10, StatusCode: 200},
	}
	opts := &options{
		slowThreshold:      500 * time.Millisecond,
		duplicateDetection: false,
	}

	result := analyze(calls, opts)
	if result.DuplicateCalls != nil {
		t.Error("expected no duplicate detection when disabled")
	}
}

func TestAnalyze_EmptyCalls(t *testing.T) {
	opts := &options{
		slowThreshold:      500 * time.Millisecond,
		duplicateDetection: true,
	}
	result := analyze(nil, opts)
	if result.SlowCalls != nil || result.FailedCalls != nil || result.DuplicateCalls != nil {
		t.Error("expected empty analysis for nil calls")
	}
}
