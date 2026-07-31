package collector

import (
	"os"
	"testing"
)

func TestEnvReaderName(t *testing.T) {
	r := NewEnvReader()
	if r.Name() != "environment" {
		t.Errorf("expected name 'environment', got %q", r.Name())
	}
}

func TestEnvReaderReadsEnvVars(t *testing.T) {
	t.Setenv("TEST_CONFIG_FOO", "bar")
	t.Setenv("TEST_CONFIG_BAZ", "qux")

	r := NewEnvReader(WithPrefixes("TEST_CONFIG_"))

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := make(map[string]string)
	for _, e := range entries {
		found[e.Key] = e.Value
	}

	if found["TEST_CONFIG_FOO"] != "bar" {
		t.Errorf("expected TEST_CONFIG_FOO=bar, got %q", found["TEST_CONFIG_FOO"])
	}
	if found["TEST_CONFIG_BAZ"] != "qux" {
		t.Errorf("expected TEST_CONFIG_BAZ=qux, got %q", found["TEST_CONFIG_BAZ"])
	}
}

func TestEnvReaderPrefixFiltering(t *testing.T) {
	t.Setenv("APP_NAME", "myapp")
	t.Setenv("APP_PORT", "8080")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("UNRELATED_VAR", "ignored")

	r := NewEnvReader(WithPrefixes("APP_"))

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, e := range entries {
		if e.Key == "DB_HOST" || e.Key == "UNRELATED_VAR" {
			t.Errorf("entry %q should not be included with prefix APP_", e.Key)
		}
	}

	found := make(map[string]string)
	for _, e := range entries {
		found[e.Key] = e.Value
	}

	if found["APP_NAME"] != "myapp" {
		t.Errorf("expected APP_NAME=myapp, got %q", found["APP_NAME"])
	}
	if found["APP_PORT"] != "8080" {
		t.Errorf("expected APP_PORT=8080, got %q", found["APP_PORT"])
	}
}

func TestEnvReaderMultiplePrefixes(t *testing.T) {
	t.Setenv("APP_NAME", "myapp")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("CACHE_TTL", "60")

	r := NewEnvReader(WithPrefixes("APP_", "DB_"))

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := make(map[string]bool)
	for _, e := range entries {
		found[e.Key] = true
	}

	if !found["APP_NAME"] {
		t.Error("expected APP_NAME to be included")
	}
	if !found["DB_HOST"] {
		t.Error("expected DB_HOST to be included")
	}
	if found["CACHE_TTL"] {
		t.Error("expected CACHE_TTL to be excluded")
	}
}

func TestEnvReaderExcludeFiltering(t *testing.T) {
	t.Setenv("TEST_ENV_KEEP", "yes")
	t.Setenv("TEST_ENV_DROP", "no")

	r := NewEnvReader(
		WithPrefixes("TEST_ENV_"),
		WithExcludes("TEST_ENV_DROP"),
	)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, e := range entries {
		if e.Key == "TEST_ENV_DROP" {
			t.Error("expected TEST_ENV_DROP to be excluded")
		}
	}

	found := false
	for _, e := range entries {
		if e.Key == "TEST_ENV_KEEP" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected TEST_ENV_KEEP to be included")
	}
}

func TestEnvReaderDefaultExcludes(t *testing.T) {
	// PATH and HOME should be excluded by default
	// These are almost always set in any environment
	r := NewEnvReader()

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, e := range entries {
		if e.Key == "PATH" || e.Key == "HOME" || e.Key == "SHELL" {
			t.Errorf("expected %q to be excluded by default", e.Key)
		}
	}
}

func TestEnvReaderWithoutDefaultExcludes(t *testing.T) {
	// Ensure PATH is in the environment
	path := os.Getenv("PATH")
	if path == "" {
		t.Skip("PATH not set in environment")
	}

	r := NewEnvReader(WithoutDefaultExcludes())

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range entries {
		if e.Key == "PATH" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected PATH to be included when default excludes are removed")
	}
}

func TestEnvReaderSortedOutput(t *testing.T) {
	t.Setenv("TEST_SORT_C", "3")
	t.Setenv("TEST_SORT_A", "1")
	t.Setenv("TEST_SORT_B", "2")

	r := NewEnvReader(WithPrefixes("TEST_SORT_"))

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) < 3 {
		t.Fatalf("expected at least 3 entries, got %d", len(entries))
	}

	for i := 1; i < len(entries); i++ {
		if entries[i].Key < entries[i-1].Key {
			t.Errorf("entries not sorted: %q came after %q", entries[i].Key, entries[i-1].Key)
		}
	}
}

func TestEnvReaderCombinedPrefixAndExclude(t *testing.T) {
	t.Setenv("MYAPP_HOST", "localhost")
	t.Setenv("MYAPP_PORT", "3000")
	t.Setenv("MYAPP_SECRET", "s3cr3t")
	t.Setenv("OTHER_VAR", "nope")

	r := NewEnvReader(
		WithPrefixes("MYAPP_"),
		WithExcludes("MYAPP_SECRET"),
	)

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := make(map[string]string)
	for _, e := range entries {
		found[e.Key] = e.Value
	}

	if found["MYAPP_HOST"] != "localhost" {
		t.Error("expected MYAPP_HOST to be included")
	}
	if found["MYAPP_PORT"] != "3000" {
		t.Error("expected MYAPP_PORT to be included")
	}
	if _, exists := found["MYAPP_SECRET"]; exists {
		t.Error("expected MYAPP_SECRET to be excluded")
	}
	if _, exists := found["OTHER_VAR"]; exists {
		t.Error("expected OTHER_VAR to be excluded (wrong prefix)")
	}
}

func TestEnvReaderEmptyValue(t *testing.T) {
	t.Setenv("TEST_EMPTY_VAL", "")

	r := NewEnvReader(WithPrefixes("TEST_EMPTY_"))

	entries, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Value != "" {
		t.Errorf("expected empty value, got %q", entries[0].Value)
	}
}
