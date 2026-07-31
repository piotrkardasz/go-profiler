package profiler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/piotrkardasz/go-profiler/collector"
)

// mockCollector is a test collector that records calls.
type mockCollector struct {
	name      string
	collected bool
	reset     bool
	data      any
	err       error
}

func (m *mockCollector) Name() string { return m.name }

func (m *mockCollector) Collect(_ context.Context, _ *http.Request, _ collector.ResponseData) (any, error) {
	m.collected = true
	return m.data, m.err
}

func (m *mockCollector) Reset() {
	m.reset = true
}

// mockLateCollector implements both Collector and LateCollector.
type mockLateCollector struct {
	mockCollector
	lateData      any
	lateErr       error
	lateCollected bool
}

func (m *mockLateCollector) LateCollect(_ context.Context) (any, error) {
	m.lateCollected = true
	return m.lateData, m.lateErr
}

// mockPanelProvider implements Collector and PanelProvider.
type mockPanelProvider struct {
	mockCollector
	panel collector.PanelMeta
}

func (m *mockPanelProvider) PanelMeta() collector.PanelMeta {
	return m.panel
}

func TestNew(t *testing.T) {
	cfg := DefaultConfig()
	p := New(cfg, nil)

	if p == nil {
		t.Fatal("expected non-nil Profiler")
	}
	if !p.IsEnabled() {
		t.Error("expected profiler to be enabled by default")
	}
	if len(p.Collectors()) != 0 {
		t.Error("expected no collectors initially")
	}
}

func TestProfilerEnabledDisabled(t *testing.T) {
	cfg := DefaultConfig()
	p := New(cfg, nil)

	if !p.IsEnabled() {
		t.Error("expected enabled")
	}

	p.SetEnabled(false)
	if p.IsEnabled() {
		t.Error("expected disabled after SetEnabled(false)")
	}

	p.SetEnabled(true)
	if !p.IsEnabled() {
		t.Error("expected enabled after SetEnabled(true)")
	}
}

func TestProfilerDisabledByConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	p := New(cfg, nil)

	if p.IsEnabled() {
		t.Error("expected profiler to be disabled")
	}
}

func TestAddCollector(t *testing.T) {
	p := New(DefaultConfig(), nil)

	c1 := &mockCollector{name: "collector1"}
	c2 := &mockCollector{name: "collector2"}

	p.AddCollector(c1)
	p.AddCollector(c2)

	collectors := p.Collectors()
	if len(collectors) != 2 {
		t.Fatalf("expected 2 collectors, got %d", len(collectors))
	}
	if collectors[0].Name() != "collector1" {
		t.Errorf("first collector name: got %q, want %q", collectors[0].Name(), "collector1")
	}
	if collectors[1].Name() != "collector2" {
		t.Errorf("second collector name: got %q, want %q", collectors[1].Name(), "collector2")
	}
}

func TestCollectProfile(t *testing.T) {
	p := New(DefaultConfig(), nil)

	c1 := &mockCollector{name: "request", data: map[string]string{"method": "GET"}}
	c2 := &mockCollector{name: "timing", data: map[string]float64{"duration_ms": 42.5}}

	p.AddCollector(c1)
	p.AddCollector(c2)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := collector.ResponseData{StatusCode: 200, Headers: http.Header{}, Size: 100}

	profile := p.CollectProfile(context.Background(), req, res)

	if profile == nil {
		t.Fatal("expected non-nil profile")
	}
	if !c1.collected {
		t.Error("collector1 was not called")
	}
	if !c2.collected {
		t.Error("collector2 was not called")
	}
	if profile.CollectorData == nil {
		t.Fatal("CollectorData is nil")
	}
	if _, ok := profile.CollectorData["request"]; !ok {
		t.Error("missing 'request' in CollectorData")
	}
	if _, ok := profile.CollectorData["timing"]; !ok {
		t.Error("missing 'timing' in CollectorData")
	}
}

func TestCollectProfileSkipsErroredCollectors(t *testing.T) {
	p := New(DefaultConfig(), nil)

	c1 := &mockCollector{name: "good", data: "ok"}
	c2 := &mockCollector{name: "bad", err: context.DeadlineExceeded}
	c3 := &mockCollector{name: "also_good", data: "fine"}

	p.AddCollector(c1)
	p.AddCollector(c2)
	p.AddCollector(c3)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := collector.ResponseData{StatusCode: 200}

	profile := p.CollectProfile(context.Background(), req, res)

	if _, ok := profile.CollectorData["good"]; !ok {
		t.Error("missing 'good' in CollectorData")
	}
	if _, ok := profile.CollectorData["bad"]; ok {
		t.Error("errored collector 'bad' should not be in CollectorData")
	}
	if _, ok := profile.CollectorData["also_good"]; !ok {
		t.Error("missing 'also_good' in CollectorData")
	}
}

func TestCollectLate(t *testing.T) {
	p := New(DefaultConfig(), nil)

	// Regular collector (not late)
	c1 := &mockCollector{name: "regular", data: "data"}
	// Late collector
	c2 := &mockLateCollector{
		mockCollector: mockCollector{name: "otel", data: "initial"},
		lateData:      map[string]string{"spans": "exported"},
	}

	p.AddCollector(c1)
	p.AddCollector(c2)

	lateData := p.CollectLate(context.Background())

	if _, ok := lateData["regular"]; ok {
		t.Error("regular collector should not appear in late data")
	}
	if _, ok := lateData["otel"]; !ok {
		t.Error("late collector 'otel' should appear in late data")
	}
	if !c2.lateCollected {
		t.Error("LateCollect was not called")
	}
}

func TestResetCollectors(t *testing.T) {
	p := New(DefaultConfig(), nil)

	c1 := &mockCollector{name: "c1"}
	c2 := &mockCollector{name: "c2"}

	p.AddCollector(c1)
	p.AddCollector(c2)

	p.ResetCollectors()

	if !c1.reset {
		t.Error("collector c1 was not reset")
	}
	if !c2.reset {
		t.Error("collector c2 was not reset")
	}
}

func TestPanelMetas(t *testing.T) {
	p := New(DefaultConfig(), nil)

	// Regular collector without PanelProvider
	c1 := &mockCollector{name: "basic"}

	// Collector with PanelProvider
	c2 := &mockPanelProvider{
		mockCollector: mockCollector{name: "custom"},
		panel: collector.PanelMeta{
			Name:      "custom",
			Label:     "Custom Panel",
			Icon:      "star",
			Component: "CustomPanel",
		},
	}

	p.AddCollector(c1)
	p.AddCollector(c2)

	metas := p.PanelMetas()

	if len(metas) != 2 {
		t.Fatalf("expected 2 panel metas, got %d", len(metas))
	}

	// Default panel for basic collector
	if metas[0].Name != "basic" {
		t.Errorf("first meta name: got %q, want %q", metas[0].Name, "basic")
	}
	if metas[0].Label != "basic" {
		t.Errorf("first meta label: got %q, want %q", metas[0].Label, "basic")
	}
	if metas[0].Icon != "code" {
		t.Errorf("first meta icon: got %q, want %q", metas[0].Icon, "code")
	}
	if metas[0].Component != "" {
		t.Errorf("first meta component: got %q, want empty", metas[0].Component)
	}

	// Custom panel
	if metas[1].Name != "custom" {
		t.Errorf("second meta name: got %q, want %q", metas[1].Name, "custom")
	}
	if metas[1].Label != "Custom Panel" {
		t.Errorf("second meta label: got %q, want %q", metas[1].Label, "Custom Panel")
	}
	if metas[1].Component != "CustomPanel" {
		t.Errorf("second meta component: got %q, want %q", metas[1].Component, "CustomPanel")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("expected Enabled to be true by default")
	}
	if cfg.StoragePath != DefaultStoragePath {
		t.Errorf("StoragePath: got %q, want %q", cfg.StoragePath, DefaultStoragePath)
	}
	if cfg.RoutePrefix != DefaultRoutePrefix {
		t.Errorf("RoutePrefix: got %q, want %q", cfg.RoutePrefix, DefaultRoutePrefix)
	}
}
