# Tasks: go-profiler Setup Tool

All tasks are to be implemented in the `github.com/piotrkardasz/go-profiler` repository.

---

## Phase 1: Build-Tag Split (Stream 1 foundation)

### Task 1.1: Remove `handler/embed.go`
Delete the current unconditional embed file that causes compilation failures when
`ui_dist/` is missing.

### Task 1.2: Create `handler/embed_ui.go` (with build tag)
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

### Task 1.3: Create `handler/embed_stub.go` (default, no tag)
```go
//go:build !profiler_ui

package handler

import "io/fs"

// UIDistFS returns nil when the profiler UI assets are not embedded.
// Use GO_PROFILER_UI_DEV=true to proxy to a Vite dev server in development.
func UIDistFS() fs.FS {
    return nil
}
```

### Task 1.4: Verify compilation without tag
Run `go build ./...` without the `profiler_ui` tag — should compile successfully
with no `ui_dist/` dependency (stub returns nil).

### Task 1.5: Verify compilation with tag
Run `go build -tags profiler_ui ./...` — should embed the committed `ui_dist/`
directory successfully.

### Task 1.6: Update existing tests
Ensure `handler/ui_test.go` and `handler/api_test.go` still pass. The UI tests
that call `UIDistFS()` should handle the nil case (skip or test 404 fallback).

---

## Phase 2: Commit Pre-Built Assets & CI Automation (Stream 1 completion)

### Task 2.1: Remove `handler/ui_dist/` from `.gitignore`
Edit `.gitignore` to remove the `handler/ui_dist/` entry. Keep `ui/dist/` and
`ui/node_modules/` ignored (those are intermediate build artifacts).

### Task 2.2: Ensure `handler/ui_dist/` is populated
If not already present, build the UI:
```bash
cd ui && npm install && npm run build
rm -rf ../handler/ui_dist
cp -r dist ../handler/ui_dist
```

### Task 2.3: Commit `handler/ui_dist/` to git
Stage and commit the pre-built assets. This is the key change that enables
zero-friction consumption for standard users.

### Task 2.4: Create `.github/workflows/build-ui.yml`
GitHub Actions workflow that automatically rebuilds and commits `handler/ui_dist/`
when `ui/` source changes are pushed to `main`:

```yaml
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

### Task 2.5: Create `.github/workflows/check-ui-freshness.yml`
PR check that warns when `handler/ui_dist/` is stale relative to `ui/` source:

```yaml
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
            echo "::warning::handler/ui_dist/ is stale. It will be auto-updated when this PR merges to main."
            echo "To update locally, run: make ui-dist"
          else
            echo "handler/ui_dist/ is up to date"
          fi
```

Configure this as a **non-required** status check (informational only). The
auto-build workflow on `main` handles the actual update after merge.

### Task 2.6: Update Makefile with local build targets
Add/update targets for local development:
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

### Task 2.7: Verify vendor workflow
In a test consumer project:
```bash
go mod vendor
go build -tags profiler_ui ./...
```
Confirm that `handler/ui_dist/` is included in the vendor tree and compilation
succeeds without any extra steps.

### Task 2.8: Verify CI workflow locally (optional)
Use [act](https://github.com/nektos/act) or push a test branch to validate:
- Push a change to `ui/` → verify auto-commit appears on `main`
- Open a PR with `ui/` changes → verify freshness check runs

---

## Phase 3: CLI Tool Implementation (Stream 2)

### Task 3.1: Create `cmd/profiler-setup/main.go` scaffold
Create the entry point with flag parsing:
- `--ui-source` flag: path to UI source directory (default: auto-resolve)
- `--output` flag: where to place built assets (required)
- `--force` flag: rebuild even if output exists
- `--verbose` flag: detailed progress

### Task 3.2: Implement flag validation
- `--output` is required — exit with usage if missing
- `--ui-source` must contain `package.json` — exit with clear error if not
- Validate paths exist and are directories

### Task 3.3: Implement prerequisite checking
Function that verifies `node` and `npm` are on PATH using `exec.LookPath`.
On Windows, also check for `npm.cmd`. Return clear error with install
instructions if missing:
```
Error: Node.js is required but not found on PATH.
Install from https://nodejs.org or via your package manager.
```

### Task 3.4: Implement default `--ui-source` resolution
When `--ui-source` is not provided:
1. Check if `./ui/package.json` exists (running from go-profiler repo)
2. Otherwise, run `go list -m -json github.com/piotrkardasz/go-profiler`
   and use `Dir` field + `/ui/`
3. If neither works, exit with error explaining the flag is required

Print the resolved path so the user knows what's being used.

### Task 3.5: Implement idempotency check
If `<output>/index.html` exists and `--force` is not set:
- Print "UI assets already present at <output>, skipping (use --force to rebuild)"
- Exit 0

### Task 3.6: Implement build logic
1. Run `npm install --silent` in the `--ui-source` directory
2. Run `npm run build` in the `--ui-source` directory
3. Remove `<output>` directory if it exists
4. Copy `<ui-source>/dist/*` to `<output>/`
5. Print success: "UI assets built and placed at <output>"

### Task 3.7: Implement cross-platform support
- Use `filepath.Join` for all paths
- On Windows: look for `npm.cmd` via `exec.LookPath`
- Use `os/exec.Command` with proper argument quoting
- Directory copy via `filepath.WalkDir` (no shell dependency)

### Task 3.8: Add error handling and messaging
- Node not found → actionable install instructions
- npm install fails → show stderr, suggest checking network/node version
- npm build fails → show stderr, suggest checking Node version compatibility
- Source dir missing package.json → "Not a valid UI source directory"
- Output parent dir doesn't exist → create it or error with guidance

---

## Phase 4: Documentation and Integration

### Task 4.1: Update `README.md` — Consumer Setup section
Document both streams:

**Standard Usage (most consumers):**
```bash
go get github.com/piotrkardasz/go-profiler
go build -tags profiler_ui ./cmd/myapp
```

**Custom Vue Panels (power users):**
```bash
# Copy the UI source
cp -r $(go list -m -json github.com/piotrkardasz/go-profiler | jq -r .Dir)/ui ./profiler-ui

# Add custom components, then build
go tool profiler-setup --ui-source=./profiler-ui --output=./handler/ui_dist
go build -tags profiler_ui ./cmd/myapp
```

**Development (no embedded UI):**
```bash
GO_PROFILER_UI_DEV=true go run ./cmd/myapp
```

### Task 4.2: Update `README.md` — Custom Panels guide
Document how to:
1. Copy the `ui/` source from the module
2. Create a custom Vue component in `src/components/panels/`
3. Register it in `src/plugin/builtin.ts`
4. Build with `profiler-setup` tool
5. Compile with `-tags profiler_ui`

### Task 4.3: Update example projects
Update examples to show the build-tag usage:
- `examples/basic/`: add comment showing `go build -tags profiler_ui`
- Remove any manual UI build steps from example READMEs

### Task 4.4: Add Go 1.24+ tool directive documentation
Document the `go get -tool` workflow:
```bash
go get -tool github.com/piotrkardasz/go-profiler/cmd/profiler-setup
go tool profiler-setup --ui-source=./my-ui --output=./handler/ui_dist
```

---

## Phase 5: Consumer Migration (existing users)

> These tasks are performed in end-user repositories after the go-profiler
> changes are released.

### Task 5.1: Bump go-profiler dependency
Update `go.mod` to the new version containing committed assets and build-tag split.

### Task 5.2: Simplify build process
- Remove `scripts/vendor-profiler-ui.sh` or equivalent workarounds
- Remove any manual `ui_dist` copy steps from Makefile/CI
- Add `-tags profiler_ui` to the `go build` command

### Task 5.3: Update Makefile (standard consumer)
```makefile
build:
	go build -tags profiler_ui ./cmd/myapp

run:
	GO_PROFILER_UI_DEV=true go run ./cmd/myapp
```

### Task 5.4: Update Makefile (power user with custom panels)
```makefile
build:
	go tool profiler-setup --ui-source=./profiler-ui --output=./vendor/github.com/piotrkardasz/go-profiler/handler/ui_dist
	go build -tags profiler_ui ./cmd/myapp

run:
	GO_PROFILER_UI_DEV=true go run ./cmd/myapp
```

### Task 5.5: Verify end-to-end
- Standard: `go build -tags profiler_ui ./cmd/myapp` produces binary with UI
- Vendor: `go mod vendor && go build -tags profiler_ui ./cmd/myapp` works
- Dev mode: `GO_PROFILER_UI_DEV=true go run ./cmd/myapp` works without assets
- Power user: custom panel visible after running profiler-setup + build
