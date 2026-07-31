package collector

// ConfigReader abstracts a source of configuration key-value pairs.
// Implement this interface to integrate any config library with the config collector.
//
// Example adapters can be built for popular libraries like:
//   - github.com/joho/godotenv
//   - github.com/spf13/viper
//   - github.com/ilyakaznacheev/cleanenv
//   - github.com/kelseyhightower/envconfig
type ConfigReader interface {
	// Name returns a human-readable identifier for this config source.
	// Used as the source label in the UI (e.g., ".env", "viper", "environment").
	Name() string

	// Read returns all configuration entries from this source.
	// Returning an empty slice with nil error is valid (source has no entries).
	// If an error is returned, the collector will skip this source gracefully.
	Read() ([]ConfigEntry, error)
}

// ConfigEntry represents a single configuration key-value pair with metadata.
type ConfigEntry struct {
	// Key is the configuration variable name.
	Key string `json:"key"`

	// Value is the configuration variable value (may be masked by the collector).
	Value string `json:"value"`

	// Source identifies where this entry came from (populated by the collector from reader Name()).
	Source string `json:"source"`

	// Default is the default value for this entry, if known by the reader.
	Default string `json:"default,omitempty"`

	// Required indicates whether this configuration entry is required.
	Required bool `json:"required,omitempty"`
}

// ConfigData is the top-level output structure returned by the config collector.
type ConfigData struct {
	// Runtime holds Go runtime information (version, OS, arch, CPUs).
	Runtime RuntimeInfo `json:"runtime"`

	// Build holds module and VCS build information.
	Build BuildInfo `json:"build"`

	// Dependencies lists all module dependencies with their versions.
	Dependencies []DependencyInfo `json:"dependencies"`

	// Sources groups configuration entries by their reader source.
	Sources []ConfigSource `json:"sources"`

	// MaskEnabled indicates whether secret masking is active.
	MaskEnabled bool `json:"mask_enabled"`
}

// RuntimeInfo holds Go runtime information.
type RuntimeInfo struct {
	// GoVersion is the Go runtime version (e.g., "go1.23.0").
	GoVersion string `json:"go_version"`

	// GOOS is the operating system target (e.g., "linux", "darwin").
	GOOS string `json:"goos"`

	// GOARCH is the architecture target (e.g., "amd64", "arm64").
	GOARCH string `json:"goarch"`

	// NumCPU is the number of logical CPUs available.
	NumCPU int `json:"num_cpu"`

	// GOMAXPROCS is the maximum number of CPUs that can execute simultaneously.
	GOMAXPROCS int `json:"gomaxprocs"`

	// Compiler is the compiler used to build the binary (e.g., "gc").
	Compiler string `json:"compiler"`
}

// BuildInfo holds module and VCS information from the Go build.
type BuildInfo struct {
	// ModulePath is the module path of the main package.
	ModulePath string `json:"module_path"`

	// GoVersion is the Go version used to build the binary.
	GoVersion string `json:"go_version"`

	// VCSRevision is the VCS commit hash.
	VCSRevision string `json:"vcs_revision"`

	// VCSTime is the VCS commit timestamp.
	VCSTime string `json:"vcs_time"`

	// VCSModified indicates whether the source tree had uncommitted changes.
	VCSModified bool `json:"vcs_modified"`
}

// DependencyInfo holds information about a single module dependency.
type DependencyInfo struct {
	// Path is the module path (e.g., "github.com/user/lib").
	Path string `json:"path"`

	// Version is the module version (e.g., "v1.2.3").
	Version string `json:"version"`
}

// ConfigSource groups configuration entries from a single reader.
type ConfigSource struct {
	// Name is the reader's identifier (matches ConfigReader.Name()).
	Name string `json:"name"`

	// Entries contains all key-value pairs from this source.
	Entries []ConfigEntry `json:"entries"`
}
