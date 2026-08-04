//go:build !profiler_ui

package handler

import "io/fs"

// UIDistFS returns nil when the profiler UI assets are not embedded.
// The UIHandler gracefully handles nil assets by returning a 404 response,
// or you can use GO_PROFILER_UI_DEV=true to proxy to a Vite dev server.
func UIDistFS() fs.FS {
	return nil
}
