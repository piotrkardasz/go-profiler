//go:build profiler_ui

package handler

import (
	"embed"
	"io/fs"
)

//go:embed all:ui_dist
var uiDistFS embed.FS

// UIDistFS returns the embedded UI filesystem, rooted at the ui_dist directory.
// Returns nil if the embedded assets are not available (e.g., not built yet).
func UIDistFS() fs.FS {
	sub, err := fs.Sub(uiDistFS, "ui_dist")
	if err != nil {
		return nil
	}
	return sub
}
