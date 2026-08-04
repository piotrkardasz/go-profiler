# Requirements: go-profiler Setup Tool

## Problem Statement

The `go-profiler` module uses `//go:embed all:ui_dist` in `handler/embed.go` to embed
a Vue-based web UI into the binary. However, `ui_dist/` is a build artifact produced by
`npm run build` in the `ui/` directory and is not committed to the repository (listed in
`.gitignore`). This means:

1. `go mod vendor` in consumer projects pulls the Go source but NOT the `ui/` directory
   (it's not Go code) and NOT `handler/ui_dist/` (it's gitignored/not committed).
2. The `//go:embed all:ui_dist` directive fails at compile time because the directory
   doesn't exist in vendor.
3. Consumers must maintain fragile scripts to manually restore the assets after every
   `go mod vendor` — there is no Go-native post-vendor hook.

## Key Insight: Two Consumer Profiles

The go-profiler UI has a **plugin registry** with a `GenericPanel` fallback. This means:

- **Standard consumers** (custom collectors that expose JSON data) do NOT need to rebuild
  the UI. The `GenericPanel` renders any collector's data as an interactive JSON tree
  automatically. Tabs appear dynamically via the `GET /api/collectors` endpoint.
- **Power users** (consumers who write custom Vue panel components for richer rendering)
  DO need to rebuild the UI with their custom components registered.

This leads to a two-stream solution.

## Goal

Provide a frictionless experience for standard consumers (zero UI build steps) while
offering a simple, explicit CLI tool for power users who customize the frontend.

---

## Stream 1: Standard Consumers (Zero Friction)

### R1: Commit Pre-Built UI Assets
The `handler/ui_dist/` directory must be committed to the repository (removed from
`.gitignore`). This ensures consumers who `go get` or `go mod vendor` the module
receive the pre-built UI assets without any additional steps.

### R2: Build-Tag Split for Embed
Replace the unconditional `//go:embed all:ui_dist` with a build-tag conditional:
- With tag `profiler_ui`: embed from `handler/ui_dist/`
- Without tag (default): `UIDistFS()` returns `nil` (UI not embedded, graceful fallback)

This allows the project to compile without the tag for development (proxies to Vite),
and with the tag for production (embedded assets).

### R3: Standard Consumer Workflow
A standard consumer's workflow should be:
```bash
go get github.com/piotrkardasz/go-profiler
go build -tags profiler_ui ./cmd/myapp
```
No Node.js, no npm, no generate steps. Custom collectors with JSON data work
automatically via GenericPanel.

### R4: Automated UI Build via GitHub Actions
A GitHub Actions workflow must automatically rebuild and commit `handler/ui_dist/`
whenever `ui/` source changes are pushed to `main`. The maintainer should NOT need
to manually build or commit UI assets. The workflow must:
1. Trigger on pushes to `main` that modify files under `ui/`
2. Build the UI (`npm ci && npm run build`)
3. Replace `handler/ui_dist/` with the fresh build output
4. Auto-commit the changes back to the repo (using `stefanzweifel/git-auto-commit-action`)
5. Use `[skip ci]` in the commit message to prevent infinite workflow loops

### R5: PR Freshness Check
A separate CI job must run on pull requests that touch `ui/` to verify that
`handler/ui_dist/` is consistent with the UI source. This catches cases where
a contributor modifies the UI but forgets to include built assets in the PR.
The check should fail with a clear message if the assets are stale.

### R6: Idempotent and Deterministic Builds
The Vite build output should be deterministic. The build workflow must clean the
output directory before rebuilding to avoid stale files. If the built output is
identical to what's already committed, the auto-commit step should be a no-op.

---

## Stream 2: Power Users (Custom Vue Panels)

### R7: CLI Tool for Custom UI Builds
The go-profiler repository must ship a Go CLI tool at `cmd/profiler-setup/main.go`
that builds the Vue UI from source and places the output into a specified directory.

### R8: Explicit Path Arguments
The tool must accept explicit flags rather than auto-detecting paths:
- `--ui-source`: path to the `ui/` source directory (defaults to the module's `ui/`)
- `--output`: path where `ui_dist/` should be placed (required, no magic detection)
- `--force`: rebuild even if output already exists
- `--verbose`: print detailed progress

### R9: No Module-Cache Magic
The tool must NOT attempt to auto-detect vendor paths or probe the module cache.
Power users who customize the UI are already in "frontend developer" territory and
can specify paths explicitly.

### R10: Prerequisites Check
If Node.js or npm is not found, the tool must exit with a clear error message
explaining what's needed and how to install it. Exit code 1.

### R11: Cross-Platform
The tool must work on Linux, macOS, and Windows (via `os/exec` and `filepath`).

### R12: Consumer Integration via go:generate
The tool should be designed so power users can add a `//go:generate` directive:
```go
//go:generate go run github.com/piotrkardasz/go-profiler/cmd/profiler-setup --ui-source=./custom-ui --output=./handler/ui_dist
```

### R13: Go 1.24+ Tool Directive Support
The tool should also work as a Go 1.24+ tool dependency:
```bash
go get -tool github.com/piotrkardasz/go-profiler/cmd/profiler-setup
go tool profiler-setup --ui-source=./custom-ui --output=./vendor/github.com/piotrkardasz/go-profiler/handler/ui_dist
```

### R14: Custom Panel Registration Workflow
The tool should support a workflow where power users:
1. Copy or fork the `ui/` source from the module
2. Add their custom Vue components
3. Register them in `ui/src/plugin/builtin.ts` (or a custom entry point)
4. Run the setup tool to build and place the output

---

## Non-Goals

- Automatic triggering after `go mod vendor` (Go doesn't support this)
- CDN/remote download of pre-built assets (unnecessary given committed ui_dist)
- Auto-detection of vendor or module cache paths (explicit is better than implicit)
- Changes to the Vue UI's plugin registry architecture
- Automatic discovery of custom Vue components (consumers register them manually)
