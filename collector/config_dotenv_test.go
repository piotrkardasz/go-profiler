package collector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDotenvReaderName(t *testing.T) {
	r := NewDotenvReader()
	if r.Name() != ".env" {
		t.Errorf("expected name '.env', got %q", r.Name())
	}

	r2 := NewDotenvReader(".env.local")
	if r2.Name() != ".env.local" {
		t.Errorf("expected name '.env.local', got %q", r2.Name())
	}

	r3 := NewDotenvReader(".env", ".env.local")
	if r3.Name() != ".env files" {
		t.Errorf("expected name '.env files', got %q", r3.Name())
	}
}

func TestDotenvReaderBasicParsing(t *testing.T) {
	content := `APP_NAME=myapp
APP_PORT=8080
DB_HOST=localhost`

	path := writeTempEnv(t, content)
	r := NewDotenvReader(path)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"APP_NAME": "myapp",
		"APP_PORT": "8080",
		"DB_HOST":  "localhost",
	}

	if len(entries) != len(expected) {
		t.Fatalf("expected %d entries, got %d", len(expected), len(entries))
	}

	for _, e := range entries {
		if want, ok := expected[e.Key]; !ok {
			t.Errorf("unexpected key %q", e.Key)
		} else if e.Value != want {
			t.Errorf("key %q: expected value %q, got %q", e.Key, want, e.Value)
		}
	}
}

func TestDotenvReaderComments(t *testing.T) {
	content := `# This is a comment
APP_NAME=myapp
# Another comment
APP_PORT=8080`

	path := writeTempEnv(t, content)
	r := NewDotenvReader(path)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestDotenvReaderBlankLines(t *testing.T) {
	content := `APP_NAME=myapp

APP_PORT=8080

`

	path := writeTempEnv(t, content)
	r := NewDotenvReader(path)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestDotenvReaderDoubleQuotes(t *testing.T) {
	content := `MSG="hello world"
MULTILINE="line1\nline2"
ESCAPED="tab\there"
BACKSLASH="path\\to\\file"
QUOTE="she said \"hi\""
`

	path := writeTempEnv(t, content)
	r := NewDotenvReader(path)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"MSG":       "hello world",
		"MULTILINE": "line1\nline2",
		"ESCAPED":   "tab\there",
		"BACKSLASH": "path\\to\\file",
		"QUOTE":     `she said "hi"`,
	}

	for _, e := range entries {
		if want, ok := expected[e.Key]; !ok {
			t.Errorf("unexpected key %q", e.Key)
		} else if e.Value != want {
			t.Errorf("key %q: expected value %q, got %q", e.Key, want, e.Value)
		}
	}
}

func TestDotenvReaderSingleQuotes(t *testing.T) {
	content := `MSG='hello world'
LITERAL='no \n escape'
INNER='has "double" quotes'`

	path := writeTempEnv(t, content)
	r := NewDotenvReader(path)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"MSG":     "hello world",
		"LITERAL": `no \n escape`,
		"INNER":   `has "double" quotes`,
	}

	for _, e := range entries {
		if want, ok := expected[e.Key]; !ok {
			t.Errorf("unexpected key %q", e.Key)
		} else if e.Value != want {
			t.Errorf("key %q: expected value %q, got %q", e.Key, want, e.Value)
		}
	}
}

func TestDotenvReaderExportPrefix(t *testing.T) {
	content := `export APP_NAME=myapp
export APP_PORT=8080
NORMAL_KEY=value`

	path := writeTempEnv(t, content)
	r := NewDotenvReader(path)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	expected := map[string]string{
		"APP_NAME":   "myapp",
		"APP_PORT":   "8080",
		"NORMAL_KEY": "value",
	}

	for _, e := range entries {
		if want, ok := expected[e.Key]; !ok {
			t.Errorf("unexpected key %q", e.Key)
		} else if e.Value != want {
			t.Errorf("key %q: expected value %q, got %q", e.Key, want, e.Value)
		}
	}
}

func TestDotenvReaderInlineComments(t *testing.T) {
	content := `APP_NAME=myapp # the app name
APP_PORT=8080 # port number
QUOTED="value # not a comment"`

	path := writeTempEnv(t, content)
	r := NewDotenvReader(path)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"APP_NAME": "myapp",
		"APP_PORT": "8080",
		"QUOTED":   "value # not a comment",
	}

	for _, e := range entries {
		if want, ok := expected[e.Key]; !ok {
			t.Errorf("unexpected key %q", e.Key)
		} else if e.Value != want {
			t.Errorf("key %q: expected value %q, got %q", e.Key, want, e.Value)
		}
	}
}

func TestDotenvReaderEqualsInValue(t *testing.T) {
	content := `DATABASE_URL=postgres://user:pass@localhost/db?sslmode=disable
BASE64=SGVsbG8gV29ybGQ=`

	path := writeTempEnv(t, content)
	r := NewDotenvReader(path)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"DATABASE_URL": "postgres://user:pass@localhost/db?sslmode=disable",
		"BASE64":       "SGVsbG8gV29ybGQ=",
	}

	for _, e := range entries {
		if want, ok := expected[e.Key]; !ok {
			t.Errorf("unexpected key %q", e.Key)
		} else if e.Value != want {
			t.Errorf("key %q: expected value %q, got %q", e.Key, want, e.Value)
		}
	}
}

func TestDotenvReaderEmptyValue(t *testing.T) {
	content := `EMPTY=
ALSO_EMPTY=''
SPACE=  `

	path := writeTempEnv(t, content)
	r := NewDotenvReader(path)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"EMPTY":      "",
		"ALSO_EMPTY": "",
		"SPACE":      "",
	}

	for _, e := range entries {
		if want, ok := expected[e.Key]; !ok {
			t.Errorf("unexpected key %q", e.Key)
		} else if e.Value != want {
			t.Errorf("key %q: expected value %q, got %q", e.Key, want, e.Value)
		}
	}
}

func TestDotenvReaderFileNotFound(t *testing.T) {
	r := NewDotenvReader("/nonexistent/path/.env")

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for missing file, got %d", len(entries))
	}
}

func TestDotenvReaderMultipleFiles(t *testing.T) {
	content1 := `APP_NAME=from_first
SHARED=first_value`

	content2 := `APP_PORT=9090
SHARED=second_value`

	path1 := writeTempEnv(t, content1)
	path2 := writeTempEnvNamed(t, content2, ".env.local")

	r := NewDotenvReader(path1, path2)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 4 entries total (both SHARED entries preserved)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	// Verify source is set correctly
	for _, e := range entries {
		if e.Source == "" {
			t.Errorf("entry %q has empty source", e.Key)
		}
	}
}

func TestDotenvReaderSource(t *testing.T) {
	content := `APP_NAME=test`

	path := writeTempEnv(t, content)
	r := NewDotenvReader(path)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Source != path {
		t.Errorf("expected source %q, got %q", path, entries[0].Source)
	}
}

// writeTempEnv creates a temporary .env file with the given content.
func writeTempEnv(t *testing.T, content string) string {
	t.Helper()
	return writeTempEnvNamed(t, content, ".env")
}

// writeTempEnvNamed creates a temporary file with the given name and content.
func writeTempEnvNamed(t *testing.T, content, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp env file: %v", err)
	}
	return path
}
