# Tasks: Config Collector

## Implementation Tasks

### Task 1: Define ConfigReader interface and data types

**Objective:** Create the interface that all config sources implement and the shared data types.

**Implementation:**
- `ConfigReader` interface with `Name() string` and `Read() ([]ConfigEntry, error)` methods
- `ConfigEntry` struct: Key, Value, Source, Default (optional), Required (optional)
- `ConfigData` struct: Runtime, Build, Dependencies, Sources, MaskEnabled
- `RuntimeInfo` struct: GoVersion, GOOS, GOARCH, NumCPU, GOMAXPROCS, Compiler
- `BuildInfo` struct: ModulePath, GoVersion, VCSRevision, VCSTime, VCSModified
- `DependencyInfo` struct: Path, Version
- `ConfigSource` struct: Name, Entries (groups entries by reader)

**Files to create:**
- `collector/config_reader.go`

---

### Task 2: Implement built-in .env file parser

**Objective:** Create a zero-dependency `.env` file parser that auto-detects and reads dotenv files.

**Implementation:**
- `DotenvReader` struct with configurable file paths
- `NewDotenvReader(paths ...string)` constructor (defaults to `.env` in working directory)
- `Name()` returns the filename (e.g., ".env")
- `Read()` opens file, parses line-by-line
- Parser handles:
  - Comment lines (`#`)
  - Blank lines (skip)
  - `export KEY=VALUE` prefix stripping
  - Unquoted values (trim trailing `#` inline comments)
  - Double-quoted values (interpret `\n`, `\r`, `\t`, `\\`, `\"`)
  - Single-quoted values (literal, no escape processing)
  - Split on first `=` only (values can contain `=`)
- If file not found (`os.ErrNotExist`), return empty slice with nil error
- Helper functions: `splitKeyValue(line)`, `unquoteValue(raw)`

**Files to create:**
- `collector/config_dotenv.go`

---

### Task 3: Implement environment variable reader

**Objective:** Create a reader that captures OS environment variables with prefix/exclude filtering.

**Implementation:**
- `EnvReader` struct with `prefixes []string` and `excludes []string`
- `NewEnvReader(opts ...EnvReaderOption)` constructor
- `EnvReaderOption` funcs: `WithPrefixes(...)`, `WithExcludes(...)`
- `Name()` returns "environment"
- `Read()`:
  - Calls `os.Environ()` to get all env vars
  - Splits each on first `=`
  - Filters by prefix (if prefixes set, only include matching)
  - Filters by exclude list (skip matching keys)
  - Sorts entries alphabetically by key
- Default excludes: PATH, HOME, SHELL, USER, LOGNAME, LANG, TERM, PWD, OLDPWD, SHLVL, _, TMPDIR

**Files to create:**
- `collector/config_env.go`

---

### Task 4: Implement ConfigCollector with options and masking

**Objective:** Create the main collector struct that orchestrates readers, caching, and secret masking.

**Implementation:**
- `ConfigCollector` struct:
  - Cached fields: `runtime RuntimeInfo`, `build BuildInfo`, `dependencies []DependencyInfo`
  - Config fields: `readers []ConfigReader`, `maskEnabled bool`, `sensitivePatterns []string`
- `NewConfigCollector(opts ...ConfigOption) *ConfigCollector`:
  - Apply options
  - Collect and cache runtime info (`runtime.Version()`, `runtime.GOOS`, etc.)
  - Collect and cache build info (`runtime/debug.ReadBuildInfo()`)
  - Check `PROFILER_MASK_SECRETS` env var for masking toggle
  - Build reader list (DotenvReader + EnvReader + user readers, unless disabled)
- `Name()` returns "config"
- `Collect(ctx, req, res)`:
  - Start with cached runtime/build/deps
  - Iterate readers, call `Read()`, populate `Source` field on each entry
  - Apply masking (if enabled): check each key against sensitive patterns
  - Skip readers that return errors (non-fatal)
  - Return `*ConfigData`
- `Reset()` — no-op
- `PanelMeta()` — returns name "config", label "Configuration", icon "settings", component "ConfigPanel"
- `shouldMask(key string) bool` — case-insensitive contains check against patterns
- Options: `configOptions` struct + all `ConfigOption` funcs from design doc
- Constants: `EnvMaskSecrets = "PROFILER_MASK_SECRETS"`, `MaskedValue = "********"`
- Default sensitive patterns list

**Files to create:**
- `collector/config.go`

---

### Task 5: Write unit tests for .env parser

**Objective:** Test all .env parsing edge cases.

**Tests:**
- `TestDotenvReaderName`: verifies name matches filename
- `TestDotenvReaderBasicParsing`: KEY=value pairs
- `TestDotenvReaderComments`: skips `#` lines
- `TestDotenvReaderBlankLines`: skips empty lines
- `TestDotenvReaderDoubleQuotes`: strips quotes, processes escapes
- `TestDotenvReaderSingleQuotes`: strips quotes, literal value
- `TestDotenvReaderExportPrefix`: strips `export ` prefix
- `TestDotenvReaderInlineComments`: trims ` # comment` from unquoted values
- `TestDotenvReaderEqualsInValue`: handles `KEY=value=with=equals`
- `TestDotenvReaderEmptyValue`: handles `KEY=` (empty string)
- `TestDotenvReaderFileNotFound`: returns empty slice, nil error
- `TestDotenvReaderMultipleFiles`: reads from multiple file paths

**Files to create:**
- `collector/config_dotenv_test.go`

---

### Task 6: Write unit tests for env reader

**Objective:** Test environment variable collection with filtering.

**Tests:**
- `TestEnvReaderName`: returns "environment"
- `TestEnvReaderReadsEnvVars`: captures set env vars
- `TestEnvReaderPrefixFiltering`: only includes vars with matching prefix
- `TestEnvReaderExcludeFiltering`: excludes specified keys
- `TestEnvReaderDefaultExcludes`: skips PATH, HOME, etc.
- `TestEnvReaderSortedOutput`: entries are alphabetically sorted
- `TestEnvReaderCombinedPrefixAndExclude`: both filters applied

**Files to create:**
- `collector/config_env_test.go`

---

### Task 7: Write unit tests for ConfigCollector

**Objective:** Test the main collector integration, masking, and options.

**Tests:**
- `TestConfigCollectorName`: returns "config"
- `TestConfigCollectorPanelMeta`: verifies all metadata fields
- `TestConfigCollectorImplementsInterfaces`: compile-time checks for Collector + PanelProvider
- `TestConfigCollectorRuntimeInfo`: verifies runtime fields populated
- `TestConfigCollectorBuildInfo`: verifies build info captured
- `TestConfigCollectorDependencies`: verifies deps list populated
- `TestConfigCollectorMaskingDisabledByDefault`: values shown in full
- `TestConfigCollectorMaskingEnabled`: sensitive keys masked when env var set
- `TestConfigCollectorCustomSensitivePatterns`: extend patterns
- `TestConfigCollectorSensitivePatternsOverride`: replace patterns
- `TestConfigCollectorWithCustomReader`: user reader integrated
- `TestConfigCollectorWithoutEnvFile`: disables dotenv reader
- `TestConfigCollectorWithoutEnvVars`: disables env reader
- `TestConfigCollectorWithoutBuildInfo`: disables build info
- `TestConfigCollectorReaderError`: skips failing reader gracefully
- `TestConfigCollectorWithEnvFilePaths`: custom .env paths used

**Files to create:**
- `collector/config_test.go`

---

### Task 8: Create UI panel component (ConfigPanel.vue)

**Objective:** Build the Vue panel that displays config collector data with tabs.

**Implementation:**
- **Summary bar**: Go version, GOOS/GOARCH, NumCPU, module path, masking status badge
- **Tab system**: Runtime, Environment, .env File, Dependencies (+ dynamic tabs for custom readers)
  - Runtime tab: key-value list of runtime + build info (Go version, OS, arch, CPUs, GOMAXPROCS, compiler, VCS revision, VCS time, module path)
  - Environment tab: searchable table (key, value) with filter input; masked values show lock icon
  - .env File tab: table (key, value) grouped by file name; masked values show lock icon
  - Dependencies tab: sortable table (module path, version)
- **Masking indicator**: badge in summary bar showing "Secrets Masked" (yellow) or "Secrets Visible" (green)
- **Search**: client-side filter input on Environment tab (filters by key name, case-insensitive)
- Dynamic tabs: any ConfigSource whose name doesn't match "environment" or start with "." gets its own tab
- TypeScript interfaces for all data types

**Files to create:**
- `ui/src/components/panels/ConfigPanel.vue`

**Files to modify:**
- `ui/src/plugin/builtin.ts` — add `registerPanel('config', ConfigPanel)` import and call

---

### Task 9: Update basic example to include config collector

**Objective:** Show config collector usage in the basic example.

**Implementation:**
- Add `collector.NewConfigCollector()` to the basic example's profiler setup
- Add a sample `.env` file to `examples/basic/` for demonstration
- Show `WithEnvPrefix` usage in a comment

**Files to modify:**
- `examples/basic/main.go`

**Files to create:**
- `examples/basic/.env`

---

### Task 10: Final verification

**Objective:** Verify all components build and test correctly.

**Verification steps:**
- `go build ./...` — root module builds without errors
- `go test ./...` — all tests pass (existing + new config collector tests)
- `go vet ./...` — no warnings
- GORM collector module still builds: `cd collector/gorm && go build ./...`
- UI TypeScript check: `cd ui && npx vue-tsc --noEmit`
- UI Vite build: `cd ui && npx vite build`
- Verify no external dependencies added to root `go.mod`
- Verify `ConfigReader` interface is exported and usable from external packages

---
