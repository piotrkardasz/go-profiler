package collector

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// DotenvReader reads configuration entries from .env files.
// It implements the ConfigReader interface with zero external dependencies.
//
// Supported syntax:
//   - KEY=value (unquoted)
//   - KEY="double quoted value" (interprets \n, \r, \t, \\, \")
//   - KEY='single quoted value' (literal, no escapes)
//   - # comment lines
//   - export KEY=value (export prefix stripped)
//   - Blank lines (skipped)
//   - Inline comments: KEY=value # comment (unquoted values only)
type DotenvReader struct {
	paths []string
}

// NewDotenvReader creates a new DotenvReader.
// If no paths are provided, it defaults to ".env" in the working directory.
func NewDotenvReader(paths ...string) *DotenvReader {
	if len(paths) == 0 {
		paths = []string{".env"}
	}
	return &DotenvReader{paths: paths}
}

// Name returns the identifier for this reader.
// If a single file is configured, returns its name (e.g., ".env").
// If multiple files are configured, returns ".env files".
func (r *DotenvReader) Name() string {
	if len(r.paths) == 1 {
		return r.paths[0]
	}
	return ".env files"
}

// Read parses all configured .env files and returns their entries.
// Files that don't exist are silently skipped (no error).
// If multiple files define the same key, the last file wins.
func (r *DotenvReader) Read() ([]ConfigEntry, error) {
	var allEntries []ConfigEntry

	for _, path := range r.paths {
		entries, err := parseDotenvFile(path)
		if err != nil {
			return nil, err
		}
		// Tag each entry with its source file
		for i := range entries {
			entries[i].Source = path
		}
		allEntries = append(allEntries, entries...)
	}

	return allEntries, nil
}

// parseDotenvFile reads and parses a single .env file.
// Returns nil, nil if the file does not exist.
func parseDotenvFile(path string) ([]ConfigEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
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
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimPrefix(line, "export ")
			line = strings.TrimSpace(line)
		}

		// Split on first '='
		key, value, ok := splitDotenvLine(line)
		if !ok {
			continue
		}

		// Unquote and clean the value
		value = unquoteDotenvValue(value)

		entries = append(entries, ConfigEntry{
			Key:   key,
			Value: value,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

// splitDotenvLine splits a line on the first '=' character.
// Returns the key, raw value, and whether the split was successful.
func splitDotenvLine(line string) (key, value string, ok bool) {
	idx := strings.IndexByte(line, '=')
	if idx < 0 {
		return "", "", false
	}

	key = strings.TrimSpace(line[:idx])
	if key == "" {
		return "", "", false
	}

	value = line[idx+1:]
	return key, value, true
}

// unquoteDotenvValue processes the raw value from a .env line.
// It handles double quotes, single quotes, and unquoted values.
func unquoteDotenvValue(raw string) string {
	raw = strings.TrimSpace(raw)

	if len(raw) == 0 {
		return ""
	}

	// Double-quoted value: interpret escape sequences
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return unescapeDoubleQuoted(raw[1 : len(raw)-1])
	}

	// Single-quoted value: literal (no escape processing)
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		return raw[1 : len(raw)-1]
	}

	// Unquoted value: trim trailing inline comment
	if idx := strings.Index(raw, " #"); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}

	return raw
}

// unescapeDoubleQuoted interprets escape sequences in a double-quoted value.
// Supported escapes: \n, \r, \t, \\, \"
func unescapeDoubleQuoted(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
			case 'r':
				b.WriteByte('\r')
				i++
			case 't':
				b.WriteByte('\t')
				i++
			case '\\':
				b.WriteByte('\\')
				i++
			case '"':
				b.WriteByte('"')
				i++
			default:
				b.WriteByte(s[i])
			}
		} else {
			b.WriteByte(s[i])
		}
	}

	return b.String()
}
