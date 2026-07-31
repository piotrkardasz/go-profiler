package profiler

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGenerateProfileID(t *testing.T) {
	t.Run("generates 16 character hex string", func(t *testing.T) {
		id, err := GenerateProfileID()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(id) != 16 {
			t.Errorf("expected ID length 16, got %d: %q", len(id), id)
		}
		// Verify it's valid hex
		for _, c := range id {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("invalid hex character %c in ID %q", c, id)
			}
		}
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 1000; i++ {
			id, err := GenerateProfileID()
			if err != nil {
				t.Fatalf("unexpected error on iteration %d: %v", i, err)
			}
			if seen[id] {
				t.Fatalf("duplicate ID generated: %q", id)
			}
			seen[id] = true
		}
	})
}

func TestProfileJSONSerialization(t *testing.T) {
	t.Run("marshal and unmarshal roundtrip", func(t *testing.T) {
		original := &Profile{
			ID:         "abc123def4567890",
			Method:     "GET",
			URL:        "/api/users?page=1",
			StatusCode: 200,
			Timestamp:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			Duration:   150 * time.Millisecond,
			CollectorData: map[string]any{
				"request": map[string]any{
					"host": "localhost",
				},
				"timing": map[string]any{
					"total_ms": 150.0,
				},
			},
		}

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}

		var restored Profile
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}

		if restored.ID != original.ID {
			t.Errorf("ID: got %q, want %q", restored.ID, original.ID)
		}
		if restored.Method != original.Method {
			t.Errorf("Method: got %q, want %q", restored.Method, original.Method)
		}
		if restored.URL != original.URL {
			t.Errorf("URL: got %q, want %q", restored.URL, original.URL)
		}
		if restored.StatusCode != original.StatusCode {
			t.Errorf("StatusCode: got %d, want %d", restored.StatusCode, original.StatusCode)
		}
		if !restored.Timestamp.Equal(original.Timestamp) {
			t.Errorf("Timestamp: got %v, want %v", restored.Timestamp, original.Timestamp)
		}
		if restored.Duration != original.Duration {
			t.Errorf("Duration: got %v, want %v", restored.Duration, original.Duration)
		}
		if restored.CollectorData == nil {
			t.Fatal("CollectorData is nil after unmarshal")
		}
		if _, ok := restored.CollectorData["request"]; !ok {
			t.Error("CollectorData missing 'request' key")
		}
		if _, ok := restored.CollectorData["timing"]; !ok {
			t.Error("CollectorData missing 'timing' key")
		}
	})

	t.Run("duration is serialized as milliseconds", func(t *testing.T) {
		p := &Profile{
			ID:            "test",
			Duration:      250 * time.Millisecond,
			Timestamp:     time.Now(),
			CollectorData: map[string]any{},
		}

		data, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}

		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("unmarshal raw error: %v", err)
		}

		duration, ok := raw["duration"].(float64)
		if !ok {
			t.Fatal("duration field is not a float64")
		}
		if duration != 250 {
			t.Errorf("duration: got %f, want 250", duration)
		}
	})
}

func TestProfileSummary(t *testing.T) {
	p := &Profile{
		ID:         "abc123",
		Method:     "POST",
		URL:        "/api/orders",
		StatusCode: 201,
		Timestamp:  time.Now(),
		Duration:   75 * time.Millisecond,
		CollectorData: map[string]any{
			"request": "some data",
		},
	}

	summary := p.Summary()

	if summary.ID != p.ID {
		t.Errorf("ID: got %q, want %q", summary.ID, p.ID)
	}
	if summary.Method != p.Method {
		t.Errorf("Method: got %q, want %q", summary.Method, p.Method)
	}
	if summary.URL != p.URL {
		t.Errorf("URL: got %q, want %q", summary.URL, p.URL)
	}
	if summary.StatusCode != p.StatusCode {
		t.Errorf("StatusCode: got %d, want %d", summary.StatusCode, p.StatusCode)
	}
	if summary.Duration != p.Duration {
		t.Errorf("Duration: got %v, want %v", summary.Duration, p.Duration)
	}
}
