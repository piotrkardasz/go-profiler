package profiler

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/piotrkardasz/go-profiler/collector"
)

// Default configuration values.
const (
	DefaultRoutePrefix = "/_profiler"
	DefaultStoragePath = "./var/profiler"
	DefaultEnvVar      = "GO_PROFILER_ENABLED"
)

// Config holds the profiler configuration.
type Config struct {
	// Enabled controls whether the profiler is active. When false, the
	// middleware passes through without collecting data.
	Enabled bool

	// StoragePath is the directory path for file-based profile storage.
	StoragePath string

	// RoutePrefix is the URL prefix for the profiler UI and API routes.
	// Defaults to "/_profiler".
	RoutePrefix string

	// Logger is the structured logger used by the profiler.
	// If nil, a default slog logger is used.
	Logger *slog.Logger

	// UIDevMode enables proxying the UI to a local Vite dev server
	// instead of serving embedded assets. Controlled by GO_PROFILER_UI_DEV env var.
	UIDevMode bool

	// UIDevServerURL is the URL of the Vite dev server when UIDevMode is true.
	// Defaults to "http://localhost:5173".
	UIDevServerURL string

	// SampleRate controls what fraction of requests are profiled (0.0 to 1.0).
	// 1.0 (default) means profile all requests. 0.1 means profile ~10%.
	// Set to < 1.0 for production use to reduce overhead.
	// Skipped requests have zero profiler overhead beyond a single float comparison.
	SampleRate float64
}

// DefaultConfig returns a Config with sensible defaults for development.
// It reads the GO_PROFILER_ENABLED env var (defaults to "true" if unset)
// and GO_PROFILER_UI_DEV env var for UI dev mode.
func DefaultConfig() Config {
	enabled := true
	if env := os.Getenv(DefaultEnvVar); env != "" {
		enabled = strings.EqualFold(env, "true") || env == "1"
	}

	uiDevMode := false
	if env := os.Getenv("GO_PROFILER_UI_DEV"); env != "" {
		uiDevMode = strings.EqualFold(env, "true") || env == "1"
	}

	return Config{
		Enabled:        enabled,
		StoragePath:    DefaultStoragePath,
		RoutePrefix:    DefaultRoutePrefix,
		Logger:         slog.Default(),
		UIDevMode:      uiDevMode,
		UIDevServerURL: "http://localhost:5173",
		SampleRate:     1.0,
	}
}

// Profiler is the central coordinator that manages collectors, storage,
// and the profiling lifecycle for HTTP requests.
type Profiler struct {
	mu         sync.RWMutex
	config     Config
	collectors []collector.Collector
	storage    Storage
	logger     *slog.Logger

	// inflight tracks the number of active async collection goroutines.
	inflight sync.WaitGroup
	// shutdownFlag indicates the profiler is shutting down. New requests
	// will skip profiling once this is set.
	shutdownFlag atomic.Bool
}

// New creates a new Profiler with the given configuration and storage backend.
func New(cfg Config, store Storage) *Profiler {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Profiler{
		config:     cfg,
		collectors: make([]collector.Collector, 0),
		storage:    store,
		logger:     logger,
	}
}

// AddCollector registers a data collector with the profiler.
// Collectors are invoked in the order they are added.
func (p *Profiler) AddCollector(c collector.Collector) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.collectors = append(p.collectors, c)
}

// Collectors returns a copy of the registered collectors slice.
func (p *Profiler) Collectors() []collector.Collector {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]collector.Collector, len(p.collectors))
	copy(result, p.collectors)
	return result
}

// IsEnabled returns whether the profiler is currently active.
func (p *Profiler) IsEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config.Enabled
}

// SetEnabled dynamically enables or disables the profiler.
func (p *Profiler) SetEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.Enabled = enabled
}

// Config returns the current profiler configuration.
func (p *Profiler) Config() Config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

// Storage returns the underlying storage backend.
func (p *Profiler) Storage() Storage {
	return p.storage
}

// IsShutdown returns whether the profiler has been shut down.
func (p *Profiler) IsShutdown() bool {
	return p.shutdownFlag.Load()
}

// Shutdown gracefully shuts down the profiler, waiting for all in-flight
// async collection goroutines to complete before returning. It respects
// the context deadline for timeout behavior. After Shutdown is called,
// new requests will skip profiling but handlers still execute normally.
func (p *Profiler) Shutdown(ctx context.Context) error {
	p.shutdownFlag.Store(true)

	done := make(chan struct{})
	go func() {
		p.inflight.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CollectProfile runs all registered collectors against the given request and
// response data, assembling a complete Profile.
func (p *Profiler) CollectProfile(ctx context.Context, req *http.Request, res collector.ResponseData) *Profile {
	p.mu.RLock()
	collectors := make([]collector.Collector, len(p.collectors))
	copy(collectors, p.collectors)
	p.mu.RUnlock()

	collectorData := make(map[string]any, len(collectors))

	for _, c := range collectors {
		data, err := c.Collect(ctx, req, res)
		if err != nil {
			p.logger.Error("collector failed",
				slog.String("collector", c.Name()),
				slog.String("error", err.Error()),
			)
			continue
		}
		collectorData[c.Name()] = data
	}

	return &Profile{
		CollectorData: collectorData,
	}
}

// CollectLate runs LateCollect on any collectors that implement LateCollector.
// It returns additional data keyed by collector name.
func (p *Profiler) CollectLate(ctx context.Context) map[string]any {
	p.mu.RLock()
	collectors := make([]collector.Collector, len(p.collectors))
	copy(collectors, p.collectors)
	p.mu.RUnlock()

	lateData := make(map[string]any)

	for _, c := range collectors {
		if lc, ok := c.(collector.LateCollector); ok {
			data, err := lc.LateCollect(ctx)
			if err != nil {
				p.logger.Error("late collector failed",
					slog.String("collector", c.Name()),
					slog.String("error", err.Error()),
				)
				continue
			}
			lateData[c.Name()] = data
		}
	}

	return lateData
}

// ResetCollectors resets all registered collectors, clearing their internal state.
func (p *Profiler) ResetCollectors() {
	p.mu.RLock()
	collectors := make([]collector.Collector, len(p.collectors))
	copy(collectors, p.collectors)
	p.mu.RUnlock()

	for _, c := range collectors {
		c.Reset()
	}
}

// PanelMetas returns the panel metadata for all registered collectors.
// Collectors implementing PanelProvider supply custom metadata; others get
// a default panel with the collector name as both name and label.
func (p *Profiler) PanelMetas() []collector.PanelMeta {
	p.mu.RLock()
	collectors := make([]collector.Collector, len(p.collectors))
	copy(collectors, p.collectors)
	p.mu.RUnlock()

	metas := make([]collector.PanelMeta, 0, len(collectors))
	for _, c := range collectors {
		if pp, ok := c.(collector.PanelProvider); ok {
			metas = append(metas, pp.PanelMeta())
		} else {
			metas = append(metas, collector.PanelMeta{
				Name:  c.Name(),
				Label: c.Name(),
				Icon:  "code",
			})
		}
	}
	return metas
}
