# Design: go-profiler Setup Tool

## Architecture Overview

```
go-profiler/
├── .github/
│   └── workflows/
│       ├── build-ui.yml           # Auto-build ui_dist on push to main
│       └── check-ui-freshness.yml # PR check: warn if ui_dist is stale
├── cmd/
│   └── profiler-setup/
│       └── main.go                # CLI tool (Stream 2: power users)
├── handler/
│   ├── embed_ui.go                # //go:build profiler_ui — real embed
│   ├── embed_stub.go              # //go:build !profiler_ui — returns nil
│   ├── ui_dist/                   # Pre-built assets (committed to git, auto-updated by CI)
│   │   ├── index.html
│   │   ├── favicon.svg
│   │   └── assets/
│   ├── ui.go                      # UIHandler (unchanged)
│   └── api.go                     # APIHandler (unchanged)
└── ui/                            # Vue source (committed, for CI + power users)
    ├── package.json
    ├── package-lock.json
    ├── vite.config.ts
    └── src/
        └── plugin/
            ├── registry.ts        # Panel registry with GenericPanel fallback
            └── builtin.ts         # Built-in panel registrations
```

---

## Stream 1: Standard Consumers

### 1.1 Commit Pre-Built UI Assets

Remove `handler/ui_dist/` from `.gitignore` and commit the pre-built assets.
This directory is small (typically <500KB of minified JS/CSS/HTML) and changes
only when the UI source is modified by the maintainer.

Consumer workflow becomes:
```bash
go get github.com/piotrkardasz/go-profiler
go build -tags profiler_ui ./cmd/myapp   # assets are already in the module
```

For vendored projects:
```bash
go mod vendor                             # ui_dist/ is .go-adjacent, gets copied
go build -tags profiler_ui ./cmd/myapp    # works out of the box
```

**Note on `go mod vendor`**: The vendor mechanism copies all files in packages
that contain `.go` files. Since `handler/` contains Go source and `ui_dist/` is
a subdirectory of `handler/`, vendor will include `ui_dist/` as long as the
`//go:embed` directive references it. This is the standard behavior for embedded
assets in vendored modules.

### 1.2 Build-Tag Split

Replace the current `handler/embed.go` with two files:

**`handler/embed_ui.go`** (compiled only with `-tags profiler_ui`):
```go
//go:build profiler_ui

package handler

import (
    "embed"
    "io/fs"
)

//go:embed all:ui_dist
var uiDistFS embed.FS

func UIDistFS() fs.FS {
    sub, err := fs.Sub(uiDistFS, "ui_dist")
    if err != nil {
        return nil
    }
    return sub
}
```

**`handler/embed_stub.go`** (compiled by default, no tag needed):
```go
//go:build !profiler_ui

package handler

import "io/fs"

// UIDistFS returns nil when the profiler UI assets are not embedded.
// The UIHandler gracefully handles nil assets by returning a 404 response,
// or you can use GO_PROFILER_UI_DEV=true to proxy to a Vite dev server.
func UIDistFS() fs.FS {
    return nil
}
```

### 1.3 Why This Works for Custom Collectors

The go-profiler UI uses a plugin registry pattern:

1. Backend: `GET /api/collectors` returns `PanelMeta` for all registered collectors
   (dynamically, at runtime).
2. Frontend: `ProfileDetail.vue` fetches this metadata and renders tabs for each
   collector that has data in the current profile.
3. Fallback: `GenericPanel` renders any collector's data as an interactive JSON
   tree when no dedicated Vue component is registered.

This means a consumer who adds custom collectors gets:
- Automatic tab appearance (runtime discovery via API)
- JSON tree rendering of their data (GenericPanel fallback)
- No UI rebuild required

### 1.4 Automated Build via GitHub Actions

The `handler/ui_dist/` directory is kept in sync automatically via CI. No manual
build or commit is needed from the maintainer.

#### Primary Workflow: Auto-build on push to `main`

```yaml
# .github/workflows/build-ui.yml
name: Build UI Assets

on:
  push:
    branches: [main]
    paths:
      - 'ui/**'

jobs:
  build-ui:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'
          cache-dependency-path: ui/package-lock.json

      - name: Install and build UI
        run: |
          cd ui
          npm ci
          npm run build

      - name: Update handler/ui_dist
        run: |
          rm -rf handler/ui_dist
          cp -r ui/dist handler/ui_dist

      - uses: stefanzweifel/git-auto-commit-action@v5
        with:
          commit_message: "chore: rebuild ui_dist assets [skip ci]"
          file_pattern: 'handler/ui_dist/**'
```

**How it works:**
1. Maintainer pushes UI changes to `main` (directly or via merged PR)
2. Workflow triggers because `ui/**` files changed
3. Builds the Vue app and replaces `handler/ui_dist/`
4. Auto-commits the result back to `main`
5. `[skip ci]` prevents the auto-commit from triggering another workflow run
6. If build output is identical (no actual changes), the action is a no-op

**Tagging for release:** Since the auto-commit lands on `main` after the UI change,
the maintainer should wait for the workflow to complete before creating a version tag.
This ensures the tagged commit includes fresh assets. The workflow takes ~1-2 minutes.

#### Secondary Workflow: PR freshness check

```yaml
# .github/workflows/check-ui-freshness.yml
name: Check UI Assets Freshness

on:
  pull_request:
    paths:
      - 'ui/**'
      - 'handler/ui_dist/**'

jobs:
  check-freshness:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'
          cache-dependency-path: ui/package-lock.json

      - name: Build UI
        run: |
          cd ui
          npm ci
          npm run build

      - name: Check ui_dist is up to date
        run: |
          rm -rf /tmp/ui_dist_check
          cp -r ui/dist /tmp/ui_dist_check
          if ! diff -r /tmp/ui_dist_check handler/ui_dist > /dev/null 2>&1; then
            echo "::error::handler/ui_dist/ is stale. It will be auto-updated when this PR merges to main."
            echo "If you need it updated in this PR, run: make ui-dist"
            exit 1
          fi
          echo "handler/ui_dist/ is up to date"
```

This check is **informational** — it warns PR reviewers that the assets will be
rebuilt after merge. It does NOT block the PR (the auto-build on `main` handles it).
Configure this as a non-required status check in branch protection rules.

#### Local Makefile (for manual/local builds)

The Makefile still provides local build targets for development and testing:

```makefile
ui-build:
	@cd ui && npm install --silent && npm run build

ui-dist: ui-build
	@rm -rf handler/ui_dist
	@cp -r ui/dist handler/ui_dist
	@echo "handler/ui_dist/ updated"

build: ui-dist
	@go build -tags profiler_ui ./...

build-dev:
	@go build ./...
```

---

## Stream 2: Power Users (Custom Vue Panels)

### 2.1 When This Is Needed

A power user needs Stream 2 when they want to:
- Write a custom Vue component for richer panel rendering (charts, tables, etc.)
- Override built-in panel components
- Add custom CSS/theming to the profiler UI

### 2.2 CLI Tool Design (`cmd/profiler-setup/main.go`)

A straightforward Go binary with explicit flags:

```
profiler-setup [flags]

Flags:
  --ui-source string   Path to the UI source directory (default: auto-resolve from module)
  --output string      Path where ui_dist/ will be placed (required)
  --force              Rebuild even if output/index.html exists
  --verbose            Print detailed progress
  --help               Show usage
```

### 2.3 Tool Behavior

```
profiler-setup --ui-source=./my-custom-ui --output=./vendor/.../handler/ui_dist

  1. Validate flags
     ├── --output is required (no magic detection)
     └── --ui-source defaults to module's ui/ if not specified

  2. Check prerequisites
     ├── node on PATH? → if not, exit 1 with install instructions
     └── npm on PATH?  → if not, exit 1 with install instructions

  3. Check idempotency (unless --force)
     └── <output>/index.html exists? → print "already present, skipping", exit 0

  4. Build the UI
     ├── cd <ui-source>
     ├── npm install --silent
     └── npm run build

  5. Copy output
     ├── rm -rf <output>
     ├── cp -r <ui-source>/dist/* → <output>/
     └── print success message, exit 0
```

### 2.4 Default `--ui-source` Resolution

When `--ui-source` is not specified, the tool resolves the module's `ui/` directory:

```go
// 1. Check if ./ui/ exists (running from the go-profiler repo itself)
// 2. Otherwise, try: go list -m -json github.com/piotrkardasz/go-profiler
//    and use the "Dir" field + "/ui/"
// 3. If neither works, error with clear guidance
```

This is a **best-effort convenience** — the tool prints what it resolved and the
user can always override with `--ui-source` explicitly. No GOFLAGS manipulation.

### 2.5 Power User Workflow

#### Option A: Fork the UI source into your project

```bash
# Copy the UI source from the module
cp -r $(go list -m -json github.com/piotrkardasz/go-profiler | jq -r .Dir)/ui ./profiler-ui

# Add your custom panel component
# Edit profiler-ui/src/plugin/builtin.ts to register it

# Build and place into vendor
go tool profiler-setup \
  --ui-source=./profiler-ui \
  --output=./vendor/github.com/piotrkardasz/go-profiler/handler/ui_dist

# Build with embedded UI
go build -tags profiler_ui ./cmd/myapp
```

#### Option B: Use go:generate

```go
// cmd/myapp/generate.go
package main

//go:generate go tool profiler-setup --ui-source=../../profiler-ui --output=../../vendor/github.com/piotrkardasz/go-profiler/handler/ui_dist
```

Then: `go generate ./cmd/myapp && go build -tags profiler_ui ./cmd/myapp`

### 2.6 Cross-Platform Support

- Use `filepath.Join` for all path construction
- On Windows, look for `npm.cmd` if `npm` is not found
- Use `os/exec.Command` with proper argument handling
- Temp directory usage via `os.MkdirTemp` if needed

---

## Consumer Decision Matrix

```
┌─────────────────────────────────────┬──────────────┬───────────────────┐
│ Use Case                            │ Stream       │ What to do        │
├─────────────────────────────────────┼──────────────┼───────────────────┤
│ Use profiler with built-in panels   │ Stream 1     │ go build -tags    │
│                                     │              │ profiler_ui       │
├─────────────────────────────────────┼──────────────┼───────────────────┤
│ Custom collector, JSON data is fine  │ Stream 1     │ Same as above     │
│ (GenericPanel renders it)           │              │ (no rebuild)      │
├─────────────────────────────────────┼──────────────┼───────────────────┤
│ Custom collector with rich Vue panel │ Stream 2     │ Fork ui/, add     │
│                                     │              │ component, run    │
│                                     │              │ profiler-setup    │
├─────────────────────────────────────┼──────────────┼───────────────────┤
│ Dev mode (hot reload)               │ Neither      │ GO_PROFILER_UI_   │
│                                     │              │ DEV=true (no tag) │
└─────────────────────────────────────┴──────────────┴───────────────────┘
```

---

## Impact on Existing Code

### Changes Required

1. **Delete** `handler/embed.go`
2. **Create** `handler/embed_ui.go` (with `profiler_ui` build tag)
3. **Create** `handler/embed_stub.go` (default, no tag)
4. **Remove** `handler/ui_dist/` from `.gitignore`
5. **Commit** `handler/ui_dist/` (already exists from prior builds)
6. **Create** `.github/workflows/build-ui.yml` (auto-build on push to main)
7. **Create** `.github/workflows/check-ui-freshness.yml` (PR freshness check)
8. **Create** `cmd/profiler-setup/main.go` (CLI tool for power users)
9. **Update** `Makefile` (add `ui-dist` target, update `build`)
10. **Update** `README.md` (document both streams)

### No Changes Required

- `handler/ui.go` — UIHandler already handles `nil` from `UIDistFS()` gracefully
- `handler/api.go` — No dependency on embed
- `collector/` — No changes
- `ui/` source — No architectural changes
- `profiler.go` — No changes
- Examples — Update build instructions only
