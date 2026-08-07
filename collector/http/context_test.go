package http

import (
	"context"
	"sync"
	"testing"
)

func TestWithContext_InitializesTracker(t *testing.T) {
	ctx := context.Background()
	ctx = WithContext(ctx)

	rc := callsFromContext(ctx)
	if rc == nil {
		t.Fatal("expected tracker to be initialized")
	}
	if len(rc.calls) != 0 {
		t.Errorf("expected empty calls, got %d", len(rc.calls))
	}
}

func TestWithContext_Idempotent(t *testing.T) {
	ctx := context.Background()
	ctx1 := WithContext(ctx)
	ctx2 := WithContext(ctx1)

	rc1 := callsFromContext(ctx1)
	rc2 := callsFromContext(ctx2)

	if rc1 != rc2 {
		t.Error("expected WithContext to be idempotent, got different trackers")
	}
}

func TestCallsFromContext_NilWhenNoTracker(t *testing.T) {
	ctx := context.Background()
	calls := CallsFromContext(ctx)
	if calls != nil {
		t.Errorf("expected nil calls for bare context, got %v", calls)
	}
}

func TestCallsFromContext_NilWhenEmpty(t *testing.T) {
	ctx := WithContext(context.Background())
	calls := CallsFromContext(ctx)
	if calls != nil {
		t.Errorf("expected nil for empty tracker, got %v", calls)
	}
}

func TestCallsFromContext_ReturnsCopy(t *testing.T) {
	ctx := WithContext(context.Background())

	appendCall(ctx, HTTPCallEntry{Service: "svc-a", Method: "GET", URL: "http://a/1"})
	appendCall(ctx, HTTPCallEntry{Service: "svc-b", Method: "POST", URL: "http://b/2"})

	calls := CallsFromContext(ctx)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}

	// Mutating the copy should not affect the internal state
	calls[0].Service = "mutated"
	original := CallsFromContext(ctx)
	if original[0].Service == "mutated" {
		t.Error("CallsFromContext returned reference to internal slice, not a copy")
	}
}

func TestAppendCall_SetsIndex(t *testing.T) {
	ctx := WithContext(context.Background())

	appendCall(ctx, HTTPCallEntry{Service: "svc", Method: "GET", URL: "http://a/1"})
	appendCall(ctx, HTTPCallEntry{Service: "svc", Method: "POST", URL: "http://a/2"})
	appendCall(ctx, HTTPCallEntry{Service: "svc", Method: "PUT", URL: "http://a/3"})

	calls := CallsFromContext(ctx)
	for i, c := range calls {
		if c.Index != i {
			t.Errorf("call %d: expected index %d, got %d", i, i, c.Index)
		}
	}
}

func TestAppendCall_NilContext(t *testing.T) {
	// Should not panic when context has no tracker
	ctx := context.Background()
	appendCall(ctx, HTTPCallEntry{Service: "svc", Method: "GET", URL: "http://a/1"})
	// No assertion needed — just verifying no panic
}

func TestAppendCall_ConcurrentSafety(t *testing.T) {
	ctx := WithContext(context.Background())
	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			appendCall(ctx, HTTPCallEntry{
				Service: "svc",
				Method:  "GET",
				URL:     "http://example.com",
			})
		}(i)
	}

	wg.Wait()

	calls := CallsFromContext(ctx)
	if len(calls) != goroutines {
		t.Errorf("expected %d calls, got %d", goroutines, len(calls))
	}

	// Verify all indices are unique
	seen := make(map[int]bool)
	for _, c := range calls {
		if seen[c.Index] {
			t.Errorf("duplicate index %d", c.Index)
		}
		seen[c.Index] = true
	}
}
