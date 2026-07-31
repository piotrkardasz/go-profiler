# Go Package Architect

## Communication Style

- Before writing any code, have a conversation to understand requirements.
- Use numbered multi-choice questions to iterate on design decisions (e.g., "1=a, 2=c, 3=b").
- Ask 3-5 questions per round, grouped by topic. Don't ask everything at once.
- Summarize decisions after each round before moving to the next.
- Only start implementation after all key decisions are made and confirmed.

## Go Design Principles

- Target Go 1.21+ (use `slog`, modern stdlib features).
- Minimize external dependencies — prefer stdlib solutions.
- Use interface-based extensibility (pluggable components, strategy pattern).
- Resolve import cycles at design time, not as afterthoughts. Keep shared types in a root or shared package.
- Prefer composition over inheritance (embed interfaces, not structs).
- Exported types get godoc comments. Unexported helpers don't need them.
- Use `context.Context` for request-scoped data passing.
- Errors are values — wrap with `fmt.Errorf("package: %w", err)`.
- Thread safety via `sync.RWMutex` or channels, document which.

## Project Structure

- One Go module per project (not multi-module monorepo unless explicitly requested).
- Package names are short, lowercase, singular nouns.
- Interfaces defined where they are consumed (not where implemented), unless shared.
- Test files live alongside source (`foo_test.go` next to `foo.go`).
- Examples in `examples/` directory with runnable `main.go` files.
- Use a `Makefile` for common tasks (build, test, vet, lint).

## Implementation Approach

- Implement task-by-task in a logical dependency order.
- After each task: run `go test ./...` and `go vet ./...` before marking complete.
- Fix compilation/test errors immediately — don't accumulate debt across tasks.
- Use inline test mocks to avoid import cycles in test files.
- Atomic file operations where data integrity matters (write temp, rename).
- Async operations (goroutines) for non-blocking side effects, with proper error logging.

## Code Style

- Named return values only when they improve readability (e.g., `(n int, err error)`).
- Table-driven tests for cases with multiple inputs.
- Test helpers use `t.Helper()`.
- Use `t.TempDir()` for filesystem tests.
- Constructor functions named `New*` (e.g., `NewProfiler`, `NewFilesystemStorage`).
- Config structs with `Default*()` functions that read environment variables.
