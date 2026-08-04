// Command profiler-setup builds the go-profiler Vue UI from source and places
// the output into a specified directory. This is intended for power users who
// write custom Vue panel components for richer rendering.
//
// Standard consumers do NOT need this tool — the pre-built handler/ui_dist/
// directory is committed to the repository and works out of the box.
//
// Usage:
//
//	profiler-setup --output=./handler/ui_dist [--ui-source=./ui] [--force] [--verbose]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	var (
		uiSource string
		output   string
		force    bool
		verbose  bool
	)

	flag.StringVar(&uiSource, "ui-source", "", "Path to the UI source directory (default: auto-resolve from module)")
	flag.StringVar(&output, "output", "", "Path where ui_dist/ will be placed (required)")
	flag.BoolVar(&force, "force", false, "Rebuild even if output/index.html exists")
	flag.BoolVar(&verbose, "verbose", false, "Print detailed progress")
	flag.Parse()

	if output == "" {
		fmt.Fprintln(os.Stderr, "Error: --output flag is required")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: profiler-setup --output=<path> [--ui-source=<path>] [--force] [--verbose]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Example:")
		fmt.Fprintln(os.Stderr, "  profiler-setup --ui-source=./my-custom-ui --output=./vendor/github.com/piotrkardasz/go-profiler/handler/ui_dist")
		os.Exit(1)
	}

	// Resolve --ui-source
	if uiSource == "" {
		resolved, err := resolveUISource(verbose)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not resolve UI source directory: %v\n", err)
			fmt.Fprintln(os.Stderr, "Please specify --ui-source explicitly.")
			os.Exit(1)
		}
		uiSource = resolved
	}

	// Validate ui-source
	pkgJSON := filepath.Join(uiSource, "package.json")
	if _, err := os.Stat(pkgJSON); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: %s does not contain a package.json\n", uiSource)
		fmt.Fprintln(os.Stderr, "Not a valid UI source directory.")
		os.Exit(1)
	}

	if verbose {
		fmt.Printf("UI source: %s\n", uiSource)
		fmt.Printf("Output:    %s\n", output)
	}

	// Check idempotency
	if !force {
		indexPath := filepath.Join(output, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			fmt.Printf("UI assets already present at %s, skipping (use --force to rebuild)\n", output)
			os.Exit(0)
		}
	}

	// Check prerequisites
	if err := checkPrerequisites(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	// Build UI
	if verbose {
		fmt.Println("Running npm install...")
	}
	if err := runCommand(uiSource, npmCommand(), "install", "--silent"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: npm install failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "Try removing node_modules and retrying.")
		os.Exit(1)
	}

	if verbose {
		fmt.Println("Running npm run build...")
	}
	if err := runCommand(uiSource, npmCommand(), "run", "build"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: npm run build failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "Check that your Node.js version is compatible (Node 18+ recommended).")
		os.Exit(1)
	}

	// Copy output
	distDir := filepath.Join(uiSource, "dist")
	if _, err := os.Stat(distDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: build did not produce %s\n", distDir)
		os.Exit(1)
	}

	// Remove existing output
	if err := os.RemoveAll(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not clean output directory: %v\n", err)
		os.Exit(1)
	}

	// Copy dist to output
	if err := copyDir(distDir, output); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not copy build output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("UI assets built and placed at %s\n", output)
}

// resolveUISource attempts to find the UI source directory automatically.
func resolveUISource(verbose bool) (string, error) {
	// 1. Check if ./ui/package.json exists (running from go-profiler repo)
	if _, err := os.Stat("ui/package.json"); err == nil {
		abs, _ := filepath.Abs("ui")
		if verbose {
			fmt.Printf("Resolved UI source from local ./ui/ directory\n")
		}
		return abs, nil
	}

	// 2. Try go list -m -json to find the module cache
	out, err := exec.Command("go", "list", "-m", "-json", "github.com/piotrkardasz/go-profiler").Output()
	if err == nil {
		var mod struct {
			Dir string `json:"Dir"`
		}
		if json.Unmarshal(out, &mod) == nil && mod.Dir != "" {
			uiDir := filepath.Join(mod.Dir, "ui")
			if _, err := os.Stat(filepath.Join(uiDir, "package.json")); err == nil {
				if verbose {
					fmt.Printf("Resolved UI source from module cache: %s\n", uiDir)
				}
				return uiDir, nil
			}
		}
	}

	return "", fmt.Errorf("could not find ui/ directory (tried ./ui/ and module cache)")
}

// checkPrerequisites verifies node and npm are available.
func checkPrerequisites() error {
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("Error: Node.js is required but not found on PATH.\nInstall from https://nodejs.org or via your package manager")
	}
	if _, err := exec.LookPath(npmCommand()); err != nil {
		return fmt.Errorf("Error: npm is required but not found on PATH.\nnpm is usually bundled with Node.js — reinstall Node.js from https://nodejs.org")
	}
	return nil
}

// npmCommand returns the npm executable name for the current platform.
func npmCommand() string {
	if runtime.GOOS == "windows" {
		// On Windows, npm is typically npm.cmd
		if _, err := exec.LookPath("npm.cmd"); err == nil {
			return "npm.cmd"
		}
	}
	return "npm"
}

// runCommand executes a command in the given directory, streaming output to stderr.
func runCommand(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Compute relative path
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		return copyFile(path, target)
	})
}

// copyFile copies a single file.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func init() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "profiler-setup — Build the go-profiler Vue UI from source")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "This tool is for power users who write custom Vue panel components.")
		fmt.Fprintln(os.Stderr, "Standard consumers do NOT need this — use 'go build -tags profiler_ui' directly.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  profiler-setup --output=<path> [--ui-source=<path>] [--force] [--verbose]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  # Build from custom UI source")
		fmt.Fprintln(os.Stderr, "  profiler-setup --ui-source=./my-custom-ui --output=./handler/ui_dist")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  # Force rebuild")
		fmt.Fprintln(os.Stderr, "  profiler-setup --output=./handler/ui_dist --force")
	}

}
