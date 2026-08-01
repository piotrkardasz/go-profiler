package collector

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

const (
	// EnvMaskSecrets is the environment variable that enables secret masking.
	// Set to "true" or "1" to mask sensitive configuration values.
	EnvMaskSecrets = "PROFILER_MASK_SECRETS"

	// MaskedValue is the replacement string used for masked secrets.
	MaskedValue = "********"
)

// defaultSensitivePatterns are the default patterns used to identify sensitive keys.
// A key is considered sensitive if its uppercased name contains any of these substrings.
var defaultSensitivePatterns = []string{
	"PASSWORD", "SECRET", "TOKEN", "KEY", "API_KEY", "APIKEY",
	"PRIVATE", "CREDENTIAL", "AUTH",
}

// configOptions holds internal configuration for the ConfigCollector.
type configOptions struct {
	readers           []ConfigReader
	envFilePaths      []string
	envFileDisabled   bool
	envVarsDisabled   bool
	buildInfoDisabled bool
	envPrefixes       []string
	envExcludes       []string
	sensitivePatterns []string
	patternsOverride  bool
}

// ConfigOption configures the ConfigCollector.
type ConfigOption func(*configOptions)

// WithReader adds a custom ConfigReader to the collector.
// Multiple readers can be added and will be executed in order.
func WithReader(r ConfigReader) ConfigOption {
	return func(o *configOptions) {
		o.readers = append(o.readers, r)
	}
}

// WithEnvFile specifies custom .env file paths to read.
// Overrides the default auto-detection of ".env" in the working directory.
func WithEnvFile(paths ...string) ConfigOption {
	return func(o *configOptions) {
		o.envFilePaths = paths
	}
}

// WithEnvPrefix configures the environment variable reader to only include
// variables whose keys start with one of the given prefixes.
func WithEnvPrefix(prefixes ...string) ConfigOption {
	return func(o *configOptions) {
		o.envPrefixes = append(o.envPrefixes, prefixes...)
	}
}

// WithEnvExclude adds specific environment variable keys to the exclude list.
func WithEnvExclude(keys ...string) ConfigOption {
	return func(o *configOptions) {
		o.envExcludes = append(o.envExcludes, keys...)
	}
}

// WithSensitivePatterns extends the default sensitive key patterns.
// Keys containing any of these substrings (case-insensitive) will be masked
// when masking is enabled via PROFILER_MASK_SECRETS.
func WithSensitivePatterns(patterns ...string) ConfigOption {
	return func(o *configOptions) {
		o.sensitivePatterns = append(o.sensitivePatterns, patterns...)
	}
}

// WithSensitivePatternsOverride replaces all default sensitive key patterns
// with the provided list.
func WithSensitivePatternsOverride(patterns ...string) ConfigOption {
	return func(o *configOptions) {
		o.sensitivePatterns = patterns
		o.patternsOverride = true
	}
}

// WithoutEnvFile disables .env file reading entirely.
func WithoutEnvFile() ConfigOption {
	return func(o *configOptions) {
		o.envFileDisabled = true
	}
}

// WithoutEnvVars disables environment variable collection entirely.
func WithoutEnvVars() ConfigOption {
	return func(o *configOptions) {
		o.envVarsDisabled = true
	}
}

// WithoutBuildInfo disables Go runtime and build info collection.
func WithoutBuildInfo() ConfigOption {
	return func(o *configOptions) {
		o.buildInfoDisabled = true
	}
}

// ConfigCollector captures application configuration, environment variables,
// .env file contents, and Go runtime/build information.
//
// It uses the ConfigReader interface to support pluggable config sources.
// Built-in readers handle .env files and OS environment variables without
// any external dependencies.
//
// Config sources are cached at construction time to avoid per-request file I/O.
// Call Refresh() to re-read all sources if configuration changes at runtime.
type ConfigCollector struct {
	// Cached at construction (immutable during process lifetime)
	runtimeInfo  RuntimeInfo
	buildInfo    BuildInfo
	dependencies []DependencyInfo

	// Cached config sources (refreshable)
	cacheMu       sync.RWMutex
	cachedSources []ConfigSource

	// Configuration
	readers           []ConfigReader
	maskEnabled       bool
	sensitivePatterns []string
	buildInfoDisabled bool
}

// NewConfigCollector creates a new ConfigCollector with the given options.
//
// By default, it reads .env files from the working directory, captures
// OS environment variables, and collects Go runtime/build information.
// Secret masking is disabled unless PROFILER_MASK_SECRETS=true is set.
func NewConfigCollector(opts ...ConfigOption) *ConfigCollector {
	// Apply options
	o := &configOptions{}
	for _, opt := range opts {
		opt(o)
	}

	// Build sensitive patterns list
	patterns := buildSensitivePatterns(o)

	// Check masking env var
	maskEnabled := isMaskEnabled()

	// Build reader list
	readers := buildReaders(o)

	c := &ConfigCollector{
		readers:           readers,
		maskEnabled:       maskEnabled,
		sensitivePatterns: patterns,
		buildInfoDisabled: o.buildInfoDisabled,
	}

	// Cache runtime and build info (doesn't change during process lifetime)
	if !o.buildInfoDisabled {
		c.runtimeInfo = collectRuntimeInfo()
		c.buildInfo, c.dependencies = collectBuildInfo()
	}

	// Cache config sources at construction to avoid per-request file I/O.
	// Call Refresh() to re-read if environment changes at runtime.
	c.cachedSources = c.readAllSources()

	return c
}

// Name returns the collector identifier.
func (c *ConfigCollector) Name() string {
	return "config"
}

// Collect gathers configuration data for the current request.
// Runtime, build info, and config sources are all cached — this method
// performs no file I/O or environment scanning.
func (c *ConfigCollector) Collect(_ context.Context, _ *http.Request, _ ResponseData) (any, error) {
	c.cacheMu.RLock()
	sources := c.cachedSources
	c.cacheMu.RUnlock()

	data := &ConfigData{
		Runtime:      c.runtimeInfo,
		Build:        c.buildInfo,
		Dependencies: c.dependencies,
		Sources:      sources,
		MaskEnabled:  c.maskEnabled,
	}

	return data, nil
}

// Reset clears internal state between requests (no-op for this collector).
func (c *ConfigCollector) Reset() {}

// Refresh re-reads all config sources and updates the cached data.
// Call this if environment variables or .env files change at runtime.
func (c *ConfigCollector) Refresh() {
	sources := c.readAllSources()
	c.cacheMu.Lock()
	c.cachedSources = sources
	c.cacheMu.Unlock()
}

// readAllSources executes all registered readers, applies masking, and
// returns the assembled config sources slice.
func (c *ConfigCollector) readAllSources() []ConfigSource {
	var sources []ConfigSource

	for _, reader := range c.readers {
		entries, err := reader.Read()
		if err != nil {
			// Non-fatal: skip this source
			continue
		}
		if len(entries) == 0 {
			continue
		}

		// Populate Source field and apply masking
		sourceName := reader.Name()
		for i := range entries {
			entries[i].Source = sourceName
			if c.shouldMask(entries[i].Key) {
				entries[i].Value = MaskedValue
			}
		}

		sources = append(sources, ConfigSource{
			Name:    sourceName,
			Entries: entries,
		})
	}

	return sources
}

// PanelMeta returns UI panel metadata for this collector.
func (c *ConfigCollector) PanelMeta() PanelMeta {
	return PanelMeta{
		Name:      "config",
		Label:     "Configuration",
		Icon:      "settings",
		Component: "ConfigPanel",
	}
}

// shouldMask checks if a key matches any sensitive pattern.
func (c *ConfigCollector) shouldMask(key string) bool {
	if !c.maskEnabled {
		return false
	}
	upper := strings.ToUpper(key)
	for _, pattern := range c.sensitivePatterns {
		if strings.Contains(upper, pattern) {
			return true
		}
	}
	return false
}

// buildSensitivePatterns constructs the final sensitive patterns list from options.
func buildSensitivePatterns(o *configOptions) []string {
	if o.patternsOverride {
		// User explicitly replaced all patterns
		patterns := make([]string, len(o.sensitivePatterns))
		for i, p := range o.sensitivePatterns {
			patterns[i] = strings.ToUpper(p)
		}
		return patterns
	}

	// Start with defaults, extend with user patterns
	patterns := make([]string, 0, len(defaultSensitivePatterns)+len(o.sensitivePatterns))
	patterns = append(patterns, defaultSensitivePatterns...)
	for _, p := range o.sensitivePatterns {
		patterns = append(patterns, strings.ToUpper(p))
	}
	return patterns
}

// isMaskEnabled checks the PROFILER_MASK_SECRETS environment variable.
func isMaskEnabled() bool {
	val := os.Getenv(EnvMaskSecrets)
	return val == "true" || val == "1"
}

// buildReaders constructs the ordered list of ConfigReaders from options.
func buildReaders(o *configOptions) []ConfigReader {
	var readers []ConfigReader

	// Add dotenv reader (unless disabled)
	if !o.envFileDisabled {
		if len(o.envFilePaths) > 0 {
			readers = append(readers, NewDotenvReader(o.envFilePaths...))
		} else {
			readers = append(readers, NewDotenvReader())
		}
	}

	// Add environment variable reader (unless disabled)
	if !o.envVarsDisabled {
		var envOpts []EnvReaderOption
		if len(o.envPrefixes) > 0 {
			envOpts = append(envOpts, WithPrefixes(o.envPrefixes...))
		}
		if len(o.envExcludes) > 0 {
			envOpts = append(envOpts, WithExcludes(o.envExcludes...))
		}
		readers = append(readers, NewEnvReader(envOpts...))
	}

	// Add user-provided readers
	readers = append(readers, o.readers...)

	return readers
}

// collectRuntimeInfo gathers Go runtime information.
func collectRuntimeInfo() RuntimeInfo {
	return RuntimeInfo{
		GoVersion:  runtime.Version(),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		NumCPU:     runtime.NumCPU(),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
		Compiler:   runtime.Compiler,
	}
}

// collectBuildInfo gathers module and VCS information from the Go build.
func collectBuildInfo() (BuildInfo, []DependencyInfo) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return BuildInfo{}, nil
	}

	info := BuildInfo{
		GoVersion: bi.GoVersion,
	}

	if bi.Main.Path != "" {
		info.ModulePath = bi.Main.Path
	}

	// Extract VCS settings from build info
	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.revision":
			info.VCSRevision = setting.Value
		case "vcs.time":
			info.VCSTime = setting.Value
		case "vcs.modified":
			info.VCSModified = setting.Value == "true"
		}
	}

	// Collect dependencies
	var deps []DependencyInfo
	for _, dep := range bi.Deps {
		deps = append(deps, DependencyInfo{
			Path:    dep.Path,
			Version: dep.Version,
		})
	}

	return info, deps
}
