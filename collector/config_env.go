package collector

import (
	"os"
	"sort"
	"strings"
)

// defaultEnvExcludes contains environment variable keys that are excluded by default
// because they are common system variables that add noise to the profiler output.
var defaultEnvExcludes = []string{
	"PATH", "HOME", "SHELL", "USER", "LOGNAME", "LANG", "TERM",
	"PWD", "OLDPWD", "SHLVL", "_", "TMPDIR", "EDITOR", "VISUAL",
	"PAGER", "LESS", "MANPATH",
}

// EnvReader reads configuration entries from OS environment variables.
// It implements the ConfigReader interface.
//
// Supports prefix filtering (only include vars matching given prefixes)
// and exclude filtering (skip specific variable names).
type EnvReader struct {
	prefixes []string
	excludes map[string]bool
}

// EnvReaderOption configures an EnvReader.
type EnvReaderOption func(*EnvReader)

// WithPrefixes configures the EnvReader to only include environment variables
// whose keys start with one of the given prefixes.
// If no prefixes are set, all variables are included (minus excludes).
func WithPrefixes(prefixes ...string) EnvReaderOption {
	return func(r *EnvReader) {
		r.prefixes = append(r.prefixes, prefixes...)
	}
}

// WithExcludes adds specific variable names to the exclude list.
// These are skipped regardless of prefix matching.
func WithExcludes(keys ...string) EnvReaderOption {
	return func(r *EnvReader) {
		for _, k := range keys {
			r.excludes[k] = true
		}
	}
}

// WithoutDefaultExcludes removes the default exclude list (PATH, HOME, etc.).
// Use this if you want to capture all system variables.
func WithoutDefaultExcludes() EnvReaderOption {
	return func(r *EnvReader) {
		r.excludes = make(map[string]bool)
	}
}

// NewEnvReader creates a new EnvReader with the given options.
// By default, common system variables (PATH, HOME, SHELL, etc.) are excluded.
func NewEnvReader(opts ...EnvReaderOption) *EnvReader {
	r := &EnvReader{
		excludes: make(map[string]bool, len(defaultEnvExcludes)),
	}

	// Apply default excludes
	for _, k := range defaultEnvExcludes {
		r.excludes[k] = true
	}

	// Apply user options
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Name returns the identifier for this reader.
func (r *EnvReader) Name() string {
	return "environment"
}

// Read captures OS environment variables, applying prefix and exclude filters.
// Results are sorted alphabetically by key.
func (r *EnvReader) Read() ([]ConfigEntry, error) {
	environ := os.Environ()
	entries := make([]ConfigEntry, 0, len(environ))

	for _, env := range environ {
		key, value, ok := splitEnvVar(env)
		if !ok {
			continue
		}

		// Skip excluded keys
		if r.excludes[key] {
			continue
		}

		// Skip keys not matching any prefix (if prefixes are set)
		if len(r.prefixes) > 0 && !matchesAnyPrefix(key, r.prefixes) {
			continue
		}

		entries = append(entries, ConfigEntry{
			Key:   key,
			Value: value,
		})
	}

	// Sort alphabetically by key for consistent output
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})

	return entries, nil
}

// splitEnvVar splits an environment variable string "KEY=VALUE" on the first '='.
func splitEnvVar(env string) (key, value string, ok bool) {
	idx := strings.IndexByte(env, '=')
	if idx < 0 {
		return "", "", false
	}
	return env[:idx], env[idx+1:], true
}

// matchesAnyPrefix checks if the key starts with any of the given prefixes.
func matchesAnyPrefix(key string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}
