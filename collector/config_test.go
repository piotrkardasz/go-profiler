package collector

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestConfigCollectorName(t *testing.T) {
	c := NewConfigCollector(WithoutEnvFile(), WithoutEnvVars())
	if c.Name() != "config" {
		t.Errorf("expected name 'config', got %q", c.Name())
	}
}

func TestConfigCollectorPanelMeta(t *testing.T) {
	c := NewConfigCollector(WithoutEnvFile(), WithoutEnvVars())
	meta := c.PanelMeta()

	if meta.Name != "config" {
		t.Errorf("expected panel name 'config', got %q", meta.Name)
	}
	if meta.Label != "Configuration" {
		t.Errorf("expected panel label 'Configuration', got %q", meta.Label)
	}
	if meta.Icon != "settings" {
		t.Errorf("expected panel icon 'settings', got %q", meta.Icon)
	}
	if meta.Component != "ConfigPanel" {
		t.Errorf("expected panel component 'ConfigPanel', got %q", meta.Component)
	}
}

func TestConfigCollectorImplementsInterfaces(t *testing.T) {
	var _ Collector = (*ConfigCollector)(nil)
	var _ PanelProvider = (*ConfigCollector)(nil)
}

func TestConfigCollectorRuntimeInfo(t *testing.T) {
	c := NewConfigCollector(WithoutEnvFile(), WithoutEnvVars())

	data := collectConfig(t, c)

	if data.Runtime.GoVersion == "" {
		t.Error("expected GoVersion to be set")
	}
	if data.Runtime.GoVersion != runtime.Version() {
		t.Errorf("expected GoVersion %q, got %q", runtime.Version(), data.Runtime.GoVersion)
	}
	if data.Runtime.GOOS != runtime.GOOS {
		t.Errorf("expected GOOS %q, got %q", runtime.GOOS, data.Runtime.GOOS)
	}
	if data.Runtime.GOARCH != runtime.GOARCH {
		t.Errorf("expected GOARCH %q, got %q", runtime.GOARCH, data.Runtime.GOARCH)
	}
	if data.Runtime.NumCPU != runtime.NumCPU() {
		t.Errorf("expected NumCPU %d, got %d", runtime.NumCPU(), data.Runtime.NumCPU)
	}
	if data.Runtime.GOMAXPROCS <= 0 {
		t.Errorf("expected GOMAXPROCS > 0, got %d", data.Runtime.GOMAXPROCS)
	}
	if data.Runtime.Compiler != runtime.Compiler {
		t.Errorf("expected Compiler %q, got %q", runtime.Compiler, data.Runtime.Compiler)
	}
}

func TestConfigCollectorBuildInfo(t *testing.T) {
	c := NewConfigCollector(WithoutEnvFile(), WithoutEnvVars())

	data := collectConfig(t, c)

	// Build info should be populated (at least GoVersion from debug.ReadBuildInfo)
	if data.Build.GoVersion == "" {
		t.Error("expected Build.GoVersion to be set")
	}
}

func TestConfigCollectorDependencies(t *testing.T) {
	c := NewConfigCollector(WithoutEnvFile(), WithoutEnvVars())

	data := collectConfig(t, c)

	// In a test binary, dependencies may or may not be present,
	// but the field should at least be non-nil if build info is available
	if data.Build.GoVersion != "" && data.Dependencies == nil {
		// This is acceptable — some build environments don't report deps
		t.Log("no dependencies reported (acceptable in test environment)")
	}
}

func TestConfigCollectorMaskingDisabledByDefault(t *testing.T) {
	// Ensure masking env var is not set
	t.Setenv(EnvMaskSecrets, "")

	c := NewConfigCollector(
		WithoutEnvFile(),
		WithoutEnvVars(),
		WithReader(&mockReader{
			name: "test",
			entries: []ConfigEntry{
				{Key: "DB_PASSWORD", Value: "supersecret"},
				{Key: "API_KEY", Value: "abc123"},
				{Key: "APP_NAME", Value: "myapp"},
			},
		}),
	)

	data := collectConfig(t, c)

	if data.MaskEnabled {
		t.Error("expected masking to be disabled by default")
	}

	entries := findSource(data, "test")
	if entries == nil {
		t.Fatal("expected 'test' source to be present")
	}

	for _, e := range entries {
		if e.Value == MaskedValue {
			t.Errorf("key %q should not be masked when masking is disabled", e.Key)
		}
	}
}

func TestConfigCollectorMaskingEnabled(t *testing.T) {
	t.Setenv(EnvMaskSecrets, "true")

	c := NewConfigCollector(
		WithoutEnvFile(),
		WithoutEnvVars(),
		WithReader(&mockReader{
			name: "test",
			entries: []ConfigEntry{
				{Key: "DB_PASSWORD", Value: "supersecret"},
				{Key: "API_KEY", Value: "abc123"},
				{Key: "MY_TOKEN", Value: "tok_xyz"},
				{Key: "APP_NAME", Value: "myapp"},
				{Key: "APP_PORT", Value: "8080"},
			},
		}),
	)

	data := collectConfig(t, c)

	if !data.MaskEnabled {
		t.Error("expected masking to be enabled")
	}

	entries := findSource(data, "test")
	if entries == nil {
		t.Fatal("expected 'test' source to be present")
	}

	maskedKeys := map[string]bool{
		"DB_PASSWORD": true,
		"API_KEY":     true,
		"MY_TOKEN":    true,
	}
	visibleKeys := map[string]bool{
		"APP_NAME": true,
		"APP_PORT": true,
	}

	for _, e := range entries {
		if maskedKeys[e.Key] && e.Value != MaskedValue {
			t.Errorf("key %q should be masked, got %q", e.Key, e.Value)
		}
		if visibleKeys[e.Key] && e.Value == MaskedValue {
			t.Errorf("key %q should not be masked", e.Key)
		}
	}
}

func TestConfigCollectorMaskingEnabledWith1(t *testing.T) {
	t.Setenv(EnvMaskSecrets, "1")

	c := NewConfigCollector(
		WithoutEnvFile(),
		WithoutEnvVars(),
		WithReader(&mockReader{
			name: "test",
			entries: []ConfigEntry{
				{Key: "SECRET_VALUE", Value: "hidden"},
			},
		}),
	)

	data := collectConfig(t, c)

	if !data.MaskEnabled {
		t.Error("expected masking to be enabled with '1'")
	}

	entries := findSource(data, "test")
	if entries[0].Value != MaskedValue {
		t.Errorf("expected masked value, got %q", entries[0].Value)
	}
}

func TestConfigCollectorCustomSensitivePatterns(t *testing.T) {
	t.Setenv(EnvMaskSecrets, "true")

	c := NewConfigCollector(
		WithoutEnvFile(),
		WithoutEnvVars(),
		WithSensitivePatterns("CONN_STRING", "DSN"),
		WithReader(&mockReader{
			name: "test",
			entries: []ConfigEntry{
				{Key: "DB_CONN_STRING", Value: "postgres://..."},
				{Key: "REDIS_DSN", Value: "redis://..."},
				{Key: "APP_NAME", Value: "myapp"},
			},
		}),
	)

	data := collectConfig(t, c)

	entries := findSource(data, "test")
	for _, e := range entries {
		switch e.Key {
		case "DB_CONN_STRING", "REDIS_DSN":
			if e.Value != MaskedValue {
				t.Errorf("key %q should be masked with custom pattern", e.Key)
			}
		case "APP_NAME":
			if e.Value == MaskedValue {
				t.Errorf("key %q should not be masked", e.Key)
			}
		}
	}
}

func TestConfigCollectorSensitivePatternsOverride(t *testing.T) {
	t.Setenv(EnvMaskSecrets, "true")

	c := NewConfigCollector(
		WithoutEnvFile(),
		WithoutEnvVars(),
		WithSensitivePatternsOverride("CUSTOM_ONLY"),
		WithReader(&mockReader{
			name: "test",
			entries: []ConfigEntry{
				{Key: "DB_PASSWORD", Value: "pass123"},       // Would match default patterns
				{Key: "MY_CUSTOM_ONLY_VAR", Value: "hidden"}, // Matches override pattern
				{Key: "APP_NAME", Value: "myapp"},
			},
		}),
	)

	data := collectConfig(t, c)

	entries := findSource(data, "test")
	for _, e := range entries {
		switch e.Key {
		case "DB_PASSWORD":
			// Default pattern "PASSWORD" is gone — should NOT be masked
			if e.Value == MaskedValue {
				t.Error("DB_PASSWORD should not be masked when patterns are overridden")
			}
		case "MY_CUSTOM_ONLY_VAR":
			if e.Value != MaskedValue {
				t.Error("MY_CUSTOM_ONLY_VAR should be masked with override pattern")
			}
		}
	}
}

func TestConfigCollectorWithCustomReader(t *testing.T) {
	c := NewConfigCollector(
		WithoutEnvFile(),
		WithoutEnvVars(),
		WithoutBuildInfo(),
		WithReader(&mockReader{
			name: "custom-source",
			entries: []ConfigEntry{
				{Key: "CUSTOM_KEY", Value: "custom_value"},
			},
		}),
	)

	data := collectConfig(t, c)

	entries := findSource(data, "custom-source")
	if entries == nil {
		t.Fatal("expected 'custom-source' to be present")
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Key != "CUSTOM_KEY" || entries[0].Value != "custom_value" {
		t.Errorf("unexpected entry: %+v", entries[0])
	}
	if entries[0].Source != "custom-source" {
		t.Errorf("expected source 'custom-source', got %q", entries[0].Source)
	}
}

func TestConfigCollectorWithoutEnvFile(t *testing.T) {
	c := NewConfigCollector(
		WithoutEnvFile(),
		WithoutEnvVars(),
		WithoutBuildInfo(),
	)

	data := collectConfig(t, c)

	// No sources should be present
	if len(data.Sources) != 0 {
		t.Errorf("expected 0 sources with everything disabled, got %d", len(data.Sources))
	}
}

func TestConfigCollectorWithoutEnvVars(t *testing.T) {
	t.Setenv("TEST_COLLECTOR_VAR", "should_not_appear")

	c := NewConfigCollector(
		WithoutEnvFile(),
		WithoutEnvVars(),
		WithReader(&mockReader{
			name: "other",
			entries: []ConfigEntry{
				{Key: "ONLY_THIS", Value: "yes"},
			},
		}),
	)

	data := collectConfig(t, c)

	if findSource(data, "environment") != nil {
		t.Error("expected 'environment' source to be absent when env vars disabled")
	}
}

func TestConfigCollectorWithoutBuildInfo(t *testing.T) {
	c := NewConfigCollector(
		WithoutEnvFile(),
		WithoutEnvVars(),
		WithoutBuildInfo(),
	)

	data := collectConfig(t, c)

	if data.Runtime.GoVersion != "" {
		t.Error("expected empty GoVersion when build info is disabled")
	}
	if data.Build.ModulePath != "" {
		t.Error("expected empty ModulePath when build info is disabled")
	}
	if data.Dependencies != nil {
		t.Error("expected nil Dependencies when build info is disabled")
	}
}

func TestConfigCollectorReaderError(t *testing.T) {
	c := NewConfigCollector(
		WithoutEnvFile(),
		WithoutEnvVars(),
		WithoutBuildInfo(),
		WithReader(&mockReader{
			name: "failing",
			err:  errors.New("read error"),
		}),
		WithReader(&mockReader{
			name: "working",
			entries: []ConfigEntry{
				{Key: "OK", Value: "yes"},
			},
		}),
	)

	data := collectConfig(t, c)

	// Failing reader should be skipped, working reader should be present
	if findSource(data, "failing") != nil {
		t.Error("expected failing reader to be skipped")
	}
	if findSource(data, "working") == nil {
		t.Error("expected working reader to be present")
	}
}

func TestConfigCollectorWithEnvFilePaths(t *testing.T) {
	path := writeTempEnv(t, "CUSTOM_FILE_VAR=hello")

	c := NewConfigCollector(
		WithEnvFile(path),
		WithoutEnvVars(),
		WithoutBuildInfo(),
	)

	data := collectConfig(t, c)

	// Should find entries from the custom path
	found := false
	for _, src := range data.Sources {
		for _, e := range src.Entries {
			if e.Key == "CUSTOM_FILE_VAR" && e.Value == "hello" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected CUSTOM_FILE_VAR from custom .env path")
	}
}

func TestConfigCollectorMultipleReaders(t *testing.T) {
	c := NewConfigCollector(
		WithoutEnvFile(),
		WithoutEnvVars(),
		WithoutBuildInfo(),
		WithReader(&mockReader{
			name:    "source-a",
			entries: []ConfigEntry{{Key: "A", Value: "1"}},
		}),
		WithReader(&mockReader{
			name:    "source-b",
			entries: []ConfigEntry{{Key: "B", Value: "2"}},
		}),
	)

	data := collectConfig(t, c)

	if len(data.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(data.Sources))
	}
	if data.Sources[0].Name != "source-a" {
		t.Errorf("expected first source 'source-a', got %q", data.Sources[0].Name)
	}
	if data.Sources[1].Name != "source-b" {
		t.Errorf("expected second source 'source-b', got %q", data.Sources[1].Name)
	}
}

func TestConfigCollectorEmptyReaderSkipped(t *testing.T) {
	c := NewConfigCollector(
		WithoutEnvFile(),
		WithoutEnvVars(),
		WithoutBuildInfo(),
		WithReader(&mockReader{
			name:    "empty",
			entries: []ConfigEntry{},
		}),
		WithReader(&mockReader{
			name:    "has-data",
			entries: []ConfigEntry{{Key: "X", Value: "Y"}},
		}),
	)

	data := collectConfig(t, c)

	// Empty reader should not appear in sources
	if len(data.Sources) != 1 {
		t.Fatalf("expected 1 source (empty skipped), got %d", len(data.Sources))
	}
	if data.Sources[0].Name != "has-data" {
		t.Errorf("expected source 'has-data', got %q", data.Sources[0].Name)
	}
}

// --- Helpers ---

// mockReader implements ConfigReader for testing.
type mockReader struct {
	name    string
	entries []ConfigEntry
	err     error
}

func (m *mockReader) Name() string                  { return m.name }
func (m *mockReader) Read() ([]ConfigEntry, error)  { return m.entries, m.err }

// collectConfig runs the collector and returns the typed ConfigData.
func collectConfig(t *testing.T, c *ConfigCollector) *ConfigData {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := ResponseData{StatusCode: 200}

	result, err := c.Collect(context.Background(), req, res)
	if err != nil {
		t.Fatalf("Collect() returned error: %v", err)
	}

	data, ok := result.(*ConfigData)
	if !ok {
		t.Fatalf("expected *ConfigData, got %T", result)
	}
	return data
}

// findSource finds a source by name in ConfigData and returns its entries.
func findSource(data *ConfigData, name string) []ConfigEntry {
	for _, src := range data.Sources {
		if src.Name == name {
			return src.Entries
		}
	}
	return nil
}
