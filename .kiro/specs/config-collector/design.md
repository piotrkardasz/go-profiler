# Design: Config Collector

## Technical Design Document

### 1. System Architecture

The config collector lives in the root module alongside the existing collectors (timing, memory, request). It captures Go runtime info, build metadata, `.env` file contents, and OS environment variables. A `ConfigReader` interface allows users to plug in adapters for external libraries without adding dependencies to the core.

```
┌──────────────────────────────────────────────────────────────────┐
│  HTTP Request                                                     │
├──────────────────────────────────────────────────────────────────┤
│  Profiler Middleware                                              │
│  └─► Application Handler                                         │
├──────────────────────────────────────────────────────────────────┤
│  After Response: Profiler calls ConfigCollector.Collect()          │
│  ├─► Return cached runtime info (Go version, OS, arch, CPU)      │
│  ├─► Return cached build info (module, VCS rev, dependencies)    │
│  ├─► Run each ConfigReader:                                      │
│  │   ├─► DotenvReader: parse .env file → []ConfigEntry           │
│  │   ├─► EnvReader: read os.Environ() → []ConfigEntry            │
│  │   └─► [User adapters]: viper/cleanenv/etc → []ConfigEntry     │
│  ├─► Apply secret masking (if PROFILER_MASK_SECRETS=true)         │
│  └─► Return ConfigData (JSON-serializable)                       │
└──────────────────────────────────────────────────────────────────┘
```

### 2. File Structure

```
collector/
├── collector.go          # Existing: Collector interface, PanelProvider, etc.
├── config.go             # ConfigCollector struct, Collect(), options, masking
├── config_reader.go      # ConfigReader interface, ConfigEntry type
├── config_dotenv.go      # Built-in zero-dep .env file parser
├── config_env.go         # Built-in OS environment variable reader
├── config_test.go        # Unit tests for ConfigCollector
├── config_dotenv_test.go # Unit tests for .env parser
├── config_env_test.go    # Unit tests for env reader
├── memory.go             # Existing
├── request.go            # Existing
└── timing.go             # Existing

ui/src/components/panels/
├── ConfigPanel.vue       # New: Vue panel for config data
├── GormPanel.vue         # Existing
├── ...
```

### 3. Core Design Decisions

#### 3.1 In Root Module (No Separate Module)

**Decision:** Place the config collector in the root `collector/` package with no external dependencies.

**Rationale:**
- The built-in `.env` parser uses only `os`, `bufio`, `strings`, `unicode` — standard library only.
- Environment variable reading uses only `os.Environ()`.
- Runtime/build info uses `runtime` and `runtime/debug` — standard library.
- No reason to isolate into a separate module since there's no dependency cost.
- Consistent with timing, memory, and request collectors.
- External adapters (godotenv, viper, cleanenv, envconfig) can be separate modules if needed, but users implement the interface in their own code.

#### 3.2 ConfigReader Interface

**Decision:** Define a minimal interface that any config source must implement.

```go
// ConfigReader abstracts a source of configuration key-value pairs.
// Implement this interface to integrate any config library with the collector.
type ConfigReader interface {
    // Name returns a human-readable identifier for this config source.
    // Used as the source label in the UI (e.g., ".env", "viper", "environment").
    Name() string

    // Read returns all configuration entries from this source.
    // Returning an empty slice with nil error is valid (source has no entries).
    Read() ([]ConfigEntry, error)
}

// ConfigEntry represents a single configuration key-value pair.
type ConfigEntry struct {
    Key      string `json:"key"`
    Value    string `json:"value"`
    Source   string `json:"source"`             // Populated by collector, matches reader Name()
    Default  string `json:"default,omitempty"`  // Default value, if known
    Required bool   `json:"required,omitempty"` // Whether this entry is required
}
```

**Rationale:**
- Minimal surface area — only two methods to implement.
- `Read()` returns `[]ConfigEntry` (not `map[string]string`) to preserve ordering and carry metadata.
- The `Source` field in `ConfigEntry` is auto-populated by the collector from `Name()`, so adapters don't need to set it.
- This design accommodates all four target libraries:
  - **godotenv**: `Read()` calls `godotenv.Read(files...)`, iterates map → entries.
  - **viper**: `Read()` calls `v.AllSettings()`, flattens nested map with dot notation → entries.
  - **envconfig**: `Read()` reflects over struct with envconfig tags, reads current values → entries.
  - **cleanenv**: `Read()` reflects over struct with `env` tags, reads current values → entries.

#### 3.3 Built-in .env Parser

**Decision:** Implement a simple `.env` parser from scratch in ~80 lines.

**Supported syntax:**
```
# Comment lines
KEY=value
KEY="double quoted value"
KEY='single quoted value'
export KEY=value
KEY=    # empty value
KEY="multi word value"
KEY='value with "quotes" inside'
KEY="value with \n escape"  # \n interpreted in double quotes
```

**Not supported (intentionally):**
- Multiline values (would require complex state machine)
- Variable interpolation (`${OTHER_VAR}`)
- Command substitution

**Rationale:**
- Covers 95%+ of real-world `.env` files.
- Zero dependencies.
- If users need advanced features (interpolation, multiline), they use `godotenv` adapter.

**Auto-detection logic:**
1. Check working directory for `.env` file.
2. If found, parse it.
3. If not found, return empty result (no error).
4. User can override with `WithEnvFile(".env.local", ".env.production")`.

#### 3.4 Environment Variable Reader

**Decision:** Built-in reader that captures OS environment variables with prefix/exclude filtering.

```go
type EnvReader struct {
    prefixes []string // If set, only include vars matching these prefixes
    excludes []string // Keys to exclude (e.g., PATH, HOME)
}
```

**Default excludes** (common noisy system vars):
```go
var defaultEnvExcludes = []string{
    "PATH", "HOME", "SHELL", "USER", "LOGNAME", "LANG", "TERM",
    "PWD", "OLDPWD", "SHLVL", "_", "TMPDIR", "EDITOR", "VISUAL",
    "PAGER", "LESS", "MANPATH", "XDG_*",
}
```

**Rationale:**
- Environment variables are the most common config source in 12-factor apps.
- Prefix filtering avoids dumping hundreds of irrelevant system vars.
- Users can pass `WithEnvPrefix("APP_", "DB_", "REDIS_")` to focus output.
- When no prefixes are set, all env vars are included (minus excludes).

#### 3.5 Secret Masking via Environment Variable

**Decision:** Masking is disabled by default. Enabled by setting `PROFILER_MASK_SECRETS=true`.

**Rationale:**
- The profiler is a **development tool** — showing values is the expected behavior.
- In staging/shared environments, teams may want masking enabled to avoid accidental exposure.
- Using an env var for the toggle means no code change needed between environments.
- Simple on/off — no complex per-key visibility rules by default.

**Masking algorithm:**
```go
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
```

**Default sensitive patterns:**
```go
var defaultSensitivePatterns = []string{
    "PASSWORD", "SECRET", "TOKEN", "KEY", "API_KEY", "APIKEY",
    "PRIVATE", "CREDENTIAL", "AUTH",
}
```

#### 3.6 Caching Strategy

**Decision:** Runtime and build info are collected once at collector creation time and cached.

```go
type ConfigCollector struct {
    // Cached at construction (never changes)
    runtime      RuntimeInfo
    build        BuildInfo
    dependencies []DependencyInfo

    // Evaluated per-request
    readers           []ConfigReader
    maskEnabled       bool
    sensitivePatterns []string
}
```

**Rationale:**
- Go version, OS, architecture, build revision don't change during process lifetime.
- Dependencies don't change at runtime.
- Environment variables and `.env` file *can* change (hot-reload, dynamic env), so they're read fresh per request.
- Keeps per-request overhead minimal.

### 4. Data Structures

```go
// ConfigData is the top-level output returned by Collect().
type ConfigData struct {
    Runtime      RuntimeInfo      `json:"runtime"`
    Build        BuildInfo        `json:"build"`
    Dependencies []DependencyInfo `json:"dependencies"`
    Sources      []ConfigSource   `json:"sources"`
    MaskEnabled  bool             `json:"mask_enabled"`
}

// RuntimeInfo holds Go runtime information.
type RuntimeInfo struct {
    GoVersion  string `json:"go_version"`
    GOOS       string `json:"goos"`
    GOARCH     string `json:"goarch"`
    NumCPU     int    `json:"num_cpu"`
    GOMAXPROCS int    `json:"gomaxprocs"`
    Compiler   string `json:"compiler"`
}

// BuildInfo holds module and VCS information from the Go build.
type BuildInfo struct {
    ModulePath string `json:"module_path"`
    GoVersion  string `json:"go_version"`
    VCSRevision string `json:"vcs_revision"`
    VCSTime     string `json:"vcs_time"`
    VCSModified bool   `json:"vcs_modified"`
}

// DependencyInfo holds a single module dependency.
type DependencyInfo struct {
    Path    string `json:"path"`
    Version string `json:"version"`
}

// ConfigSource groups config entries by their source reader.
type ConfigSource struct {
    Name    string        `json:"name"`
    Entries []ConfigEntry `json:"entries"`
}
```

### 5. Options Design

```go
type configOptions struct {
    readers           []ConfigReader
    envFilePaths      []string   // Override auto-detect
    envFileDisabled   bool
    envVarsDisabled   bool
    buildInfoDisabled bool
    envPrefixes       []string
    envExcludes       []string
    sensitivePatterns []string
    patternsOverride  bool       // true = replace defaults, false = extend
}

type ConfigOption func(*configOptions)

func WithReader(r ConfigReader) ConfigOption { ... }
func WithEnvFile(paths ...string) ConfigOption { ... }
func WithEnvPrefix(prefixes ...string) ConfigOption { ... }
func WithEnvExclude(keys ...string) ConfigOption { ... }
func WithSensitivePatterns(patterns ...string) ConfigOption { ... }
func WithSensitivePatternsOverride(patterns ...string) ConfigOption { ... }
func WithoutEnvFile() ConfigOption { ... }
func WithoutEnvVars() ConfigOption { ... }
func WithoutBuildInfo() ConfigOption { ... }
```

### 6. Collector Lifecycle

```go
func NewConfigCollector(opts ...ConfigOption) *ConfigCollector {
    // 1. Apply options
    // 2. Collect runtime info (cached)
    // 3. Collect build info (cached)
    // 4. Resolve mask setting from PROFILER_MASK_SECRETS env var
    // 5. Build internal reader list:
    //    - DotenvReader (if not disabled)
    //    - EnvReader (if not disabled)
    //    - User-provided readers
    return collector
}

func (c *ConfigCollector) Name() string { return "config" }

func (c *ConfigCollector) Collect(ctx context.Context, req *http.Request, res ResponseData) (any, error) {
    // 1. Start with cached runtime + build + deps
    // 2. Run each reader, collect entries, populate Source field
    // 3. Apply masking to all entries (if enabled)
    // 4. Return ConfigData
}

func (c *ConfigCollector) Reset() {} // No-op, stateless per request

func (c *ConfigCollector) PanelMeta() PanelMeta {
    return PanelMeta{
        Name:      "config",
        Label:     "Configuration",
        Icon:      "settings",
        Component: "ConfigPanel",
    }
}
```

### 7. .env Parser Design

```go
// parseDotenv reads a .env file and returns key-value pairs.
// It handles comments, quotes, export prefix, and blank lines.
func parseDotenv(path string) ([]ConfigEntry, error) {
    file, err := os.Open(path)
    if errors.Is(err, os.ErrNotExist) {
        return nil, nil // File not found = empty result, no error
    }
    if err != nil {
        return nil, err
    }
    defer file.Close()

    var entries []ConfigEntry
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        // Skip empty lines and comments
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        // Strip optional "export " prefix
        line = strings.TrimPrefix(line, "export ")
        // Split on first '='
        key, value := splitKeyValue(line)
        if key == "" {
            continue
        }
        // Unquote value
        value = unquoteValue(value)
        entries = append(entries, ConfigEntry{Key: key, Value: value})
    }
    return entries, scanner.Err()
}
```

**Quote handling:**
- Double quotes: strip quotes, interpret `\n`, `\r`, `\t`, `\\`, `\"`
- Single quotes: strip quotes, literal value (no escape processing)
- No quotes: trim trailing inline comments (` #`), trim whitespace

### 8. Adapter Examples (Documentation, Not Code in Repo)

Users implement the interface in their application code:

**godotenv adapter:**
```go
import "github.com/joho/godotenv"

type GodotenvReader struct {
    files []string
}

func (r *GodotenvReader) Name() string { return "godotenv" }

func (r *GodotenvReader) Read() ([]collector.ConfigEntry, error) {
    envMap, err := godotenv.Read(r.files...)
    if err != nil {
        return nil, err
    }
    entries := make([]collector.ConfigEntry, 0, len(envMap))
    for k, v := range envMap {
        entries = append(entries, collector.ConfigEntry{Key: k, Value: v})
    }
    return entries, nil
}
```

**viper adapter:**
```go
import "github.com/spf13/viper"

type ViperReader struct {
    v *viper.Viper
}

func (r *ViperReader) Name() string { return "viper" }

func (r *ViperReader) Read() ([]collector.ConfigEntry, error) {
    entries := make([]collector.ConfigEntry, 0)
    for k, v := range flatten(r.v.AllSettings(), "") {
        entries = append(entries, collector.ConfigEntry{Key: k, Value: fmt.Sprint(v)})
    }
    return entries, nil
}
```

**cleanenv adapter:**
```go
type CleanenvReader struct {
    cfg interface{} // Pointer to user's config struct
}

func (r *CleanenvReader) Name() string { return "cleanenv" }

func (r *CleanenvReader) Read() ([]collector.ConfigEntry, error) {
    // Use reflection to read struct tags and current field values
    // Extract "env" tags to get key names, reflect field values
    return extractStructEntries(r.cfg, "env")
}
```

**envconfig adapter:**
```go
type EnvconfigReader struct {
    prefix string
    spec   interface{} // Pointer to user's config struct
}

func (r *EnvconfigReader) Name() string { return "envconfig" }

func (r *EnvconfigReader) Read() ([]collector.ConfigEntry, error) {
    // Use reflection to read struct tags and current field values
    // Extract "envconfig" tags or auto-generated uppercase key names
    return extractStructEntries(r.spec, "envconfig")
}
```

### 9. UI Panel Design

**ConfigPanel.vue** structure:

```
┌─────────────────────────────────────────────────────────────────┐
│ Summary Bar                                                      │
│ [Go 1.23] [linux/amd64] [8 CPUs] [module: github.com/user/app] │
│ [Masking: OFF]                                                   │
├─────────────────────────────────────────────────────────────────┤
│ [Runtime] [Environment] [.env File] [Dependencies]               │
├─────────────────────────────────────────────────────────────────┤
│ Runtime Tab:                                                     │
│ ┌──────────────────────────────────────────────────────────────┐│
│ │ Go Version     go1.23.0                                      ││
│ │ OS / Arch      linux / amd64                                 ││
│ │ CPUs           8 (GOMAXPROCS: 8)                             ││
│ │ Compiler       gc                                            ││
│ │ VCS Revision   a1b2c3d4e5f6 (dirty: no)                     ││
│ │ VCS Time       2024-01-15T10:30:00Z                          ││
│ │ Module         github.com/user/myapp                         ││
│ └──────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────┤
│ Environment Tab:                                                 │
│ ┌─ Search: [________________] ─────────────────────────────────┐│
│ │ Key              │ Value              │ Source                ││
│ │ APP_PORT         │ 8080               │ environment           ││
│ │ DB_HOST          │ localhost           │ environment           ││
│ │ DB_PASSWORD      │ ********           │ environment  🔒       ││
│ │ REDIS_URL        │ redis://localhost   │ environment           ││
│ └──────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────┤
│ .env File Tab:                                                   │
│ ┌─ Source: .env ───────────────────────────────────────────────┐│
│ │ Key              │ Value                                      ││
│ │ APP_NAME         │ my-application                             ││
│ │ APP_DEBUG        │ true                                       ││
│ │ DB_CONNECTION    │ postgres://user:pass@localhost/db           ││
│ └──────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────┤
│ Dependencies Tab:                                                │
│ ┌──────────────────────────────────────────────────────────────┐│
│ │ Module Path                          │ Version               ││
│ │ github.com/piotrkardasz/go-profiler  │ v0.3.0                ││
│ │ gorm.io/gorm                         │ v1.25.12              ││
│ │ golang.org/x/net                     │ v0.20.0               ││
│ └──────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

**Tab logic:**
- Runtime tab: always shown.
- Environment tab: shown if `Sources` contains a reader named "environment".
- .env File tab: shown if `Sources` contains a reader with name starting with "." (e.g., ".env", ".env.local").
- Dependencies tab: shown if `Dependencies` array is non-empty.
- Additional user readers: shown as extra tabs with the reader's `Name()`.

**Search/filter on Environment tab:**
- Client-side filter on key name (case-insensitive substring match).
- No backend changes needed.

### 10. Integration Points

1. **Collector registration** — users add it like any other collector:
   ```go
   p.AddCollector(collector.NewConfigCollector(
       collector.WithEnvPrefix("APP_", "DB_"),
   ))
   ```

2. **Profile data** — stored in `Profile.CollectorData["config"]` as `*ConfigData`.

3. **API** — served via existing `GET /api/profiles/{id}` endpoint, no changes needed.

4. **UI panel** — registered in `builtin.ts`:
   ```ts
   import ConfigPanel from '../components/panels/ConfigPanel.vue'
   registerPanel('config', ConfigPanel)
   ```

5. **Panel metadata** — returned via `GET /api/collectors` endpoint automatically.

### 11. Error Handling

- If a `ConfigReader.Read()` returns an error, the collector logs it internally and skips that source (doesn't fail the entire collection).
- If `.env` file doesn't exist, returns empty entries (not an error).
- If `runtime/debug.ReadBuildInfo()` returns false, build info fields are empty strings.
- All errors are non-fatal — the collector always returns a valid `ConfigData`.
