# Requirements: Config Collector

## Overview

Add a configuration collector to the go-profiler that captures application runtime configuration, environment variables, Go build information, and `.env` file contents per request. The collector uses a `ConfigReader` interface to abstract config sources, ships with a built-in `.env` file reader (no external dependencies), and allows users to plug in adapters for popular libraries (godotenv, viper, cleanenv, envconfig). Secret values are shown by default but can be masked via an environment variable opt-in.

## Functional Requirements

### FR-1: Core Module (No External Dependencies)

- FR-1.1: The config collector MUST live in the root module under `collector/config.go` (alongside timing, memory, request collectors).
- FR-1.2: The collector MUST NOT introduce any external Go dependencies — only standard library.
- FR-1.3: The built-in `.env` file parser MUST handle the standard dotenv format (KEY=VALUE, comments with `#`, quoted values, blank lines).
- FR-1.4: The collector MUST implement the `collector.Collector` interface.
- FR-1.5: The collector MUST implement the `collector.PanelProvider` interface.

### FR-2: ConfigReader Interface

- FR-2.1: The collector MUST define a `ConfigReader` interface that any config source can implement.
- FR-2.2: The interface MUST expose:
  ```go
  type ConfigReader interface {
      Name() string
      Read() ([]ConfigEntry, error)
  }
  ```
- FR-2.3: `ConfigEntry` MUST contain: `Key`, `Value`, `Source` (origin identifier), and optionally `Default` and `Required` fields.
- FR-2.4: The collector MUST support multiple `ConfigReader` implementations registered simultaneously.
- FR-2.5: Users MUST be able to create adapters for external libraries (godotenv, viper, cleanenv, envconfig) by implementing the `ConfigReader` interface.

### FR-3: Built-in .env File Reader

- FR-3.1: The collector MUST include a zero-dependency `.env` file reader as the default `ConfigReader`.
- FR-3.2: The built-in reader MUST auto-detect `.env` files by searching the working directory.
- FR-3.3: The reader MUST support configurable file paths (e.g., `.env.local`, `.env.production`).
- FR-3.4: The reader MUST handle: unquoted values, single-quoted values, double-quoted values, `#` comments, empty lines, `export` prefix.
- FR-3.5: If no `.env` file is found, the reader MUST return an empty result without error.
- FR-3.6: The reader MUST set `Source` to the filename (e.g., ".env", ".env.local").

### FR-4: Environment Variables Collection

- FR-4.1: The collector MUST include a built-in reader that captures OS environment variables.
- FR-4.2: By default, the environment reader MUST capture ALL environment variables.
- FR-4.3: The environment reader MUST support filtering via a prefix list (e.g., only `APP_`, `DB_`, `REDIS_` prefixed vars).
- FR-4.4: The environment reader MUST support an exclude list for variables to skip (e.g., `PATH`, `HOME`, `SHELL`).
- FR-4.5: The environment reader MUST set `Source` to "environment".

### FR-5: Go Runtime & Build Information

- FR-5.1: The collector MUST capture Go runtime information: Go version, GOOS, GOARCH, NumCPU, GOMAXPROCS, Compiler.
- FR-5.2: The collector MUST capture build info via `runtime/debug.ReadBuildInfo()`: module path, Go version, VCS revision, VCS time.
- FR-5.3: The collector MUST capture module dependencies (name + version) from build info.
- FR-5.4: Runtime and build info MUST be collected once (it doesn't change between requests) and cached.

### FR-6: Secret Masking

- FR-6.1: By default, ALL config values MUST be shown unmasked (the profiler is a development tool).
- FR-6.2: Secret masking MUST be enabled by setting the environment variable `PROFILER_MASK_SECRETS=true` (or `1`).
- FR-6.3: When masking is enabled, values for keys matching sensitive patterns MUST be replaced with `"********"`.
- FR-6.4: Default sensitive patterns MUST include (case-insensitive): `PASSWORD`, `SECRET`, `TOKEN`, `KEY`, `API_KEY`, `APIKEY`, `PRIVATE`, `CREDENTIAL`, `AUTH`.
- FR-6.5: Users MUST be able to extend the sensitive patterns list via option function.
- FR-6.6: Users MUST be able to replace the default patterns entirely.
- FR-6.7: The masking MUST apply uniformly across all `ConfigReader` sources.

### FR-7: Configuration Options

- FR-7.1: The collector MUST use functional options pattern consistent with other collectors.
- FR-7.2: Options MUST include:
  - `WithReader(reader ConfigReader)` — add a custom config reader
  - `WithEnvFile(paths ...string)` — specify `.env` file paths (overrides auto-detect)
  - `WithEnvPrefix(prefixes ...string)` — filter environment variables by prefix
  - `WithEnvExclude(keys ...string)` — exclude specific environment variable keys
  - `WithSensitivePatterns(patterns ...string)` — extend sensitive key patterns
  - `WithSensitivePatternsOverride(patterns ...string)` — replace all sensitive patterns
  - `WithoutEnvFile()` — disable `.env` file reading
  - `WithoutEnvVars()` — disable environment variable collection
  - `WithoutBuildInfo()` — disable build/runtime info collection

### FR-8: Collected Data Structure

- FR-8.1: The collector output MUST be JSON-serializable.
- FR-8.2: The output MUST be structured as:
  ```go
  type ConfigData struct {
      Runtime      RuntimeInfo     `json:"runtime"`
      Build        BuildInfo       `json:"build"`
      Dependencies []DependencyInfo `json:"dependencies"`
      Sources      []ConfigSource  `json:"sources"`
      MaskEnabled  bool            `json:"mask_enabled"`
  }
  ```
- FR-8.3: `Sources` MUST group config entries by their reader name (e.g., ".env", "environment").
- FR-8.4: Each `ConfigSource` MUST contain the reader name and its list of `ConfigEntry` items.

### FR-9: UI Panel

- FR-9.1: The collector MUST have a custom Vue panel component (`ConfigPanel`) registered as "config".
- FR-9.2: The panel MUST display a summary bar with: Go version, GOOS/GOARCH, NumCPU, module path.
- FR-9.3: The panel MUST have tabbed navigation: Runtime, Environment, .env File, Dependencies.
- FR-9.4: The Runtime tab MUST display Go version, OS, architecture, CPUs, GOMAXPROCS, compiler, VCS revision, VCS time.
- FR-9.5: The Environment tab MUST display a searchable/filterable table of environment variables (key, value, source).
- FR-9.6: The .env File tab MUST display parsed `.env` file entries (key, value) with the filename shown.
- FR-9.7: The Dependencies tab MUST display a table of module dependencies (path, version).
- FR-9.8: Masked values MUST display as `********` with a visual indicator (lock icon or badge).
- FR-9.9: The panel MUST show a "masking enabled" or "masking disabled" badge in the summary bar.
- FR-9.10: The Environment tab MUST support filtering/searching by key name.

## Non-Functional Requirements

### NFR-1: Performance

- NFR-1.1: The collector MUST cache runtime and build info (collected once at startup, reused across requests).
- NFR-1.2: Environment variables and `.env` reading MUST happen once per request cycle during `Collect()`.
- NFR-1.3: The collector MUST add negligible overhead (<1ms) to request processing.

### NFR-2: Compatibility

- NFR-2.1: MUST work with Go 1.21+ (matching root module requirement).
- NFR-2.2: MUST NOT break existing collectors or middleware chain.
- NFR-2.3: The `ConfigReader` interface MUST be stable for third-party adapter implementation.

### NFR-3: Extensibility

- NFR-3.1: Third-party adapters (viper, godotenv, cleanenv, envconfig) MUST be implementable without modifying the collector package.
- NFR-3.2: The interface design MUST accommodate config sources that read from files, environment, remote stores, or structs.
- NFR-3.3: Future adapters MAY be published as separate modules (e.g., `collector/config/viper`) if they introduce dependencies.

### NFR-4: Security

- NFR-4.1: The masking feature MUST be purely opt-in via environment variable — default is to show all values.
- NFR-4.2: The collector MUST NOT modify or set any environment variables.
- NFR-4.3: The collector MUST NOT write to any files.
