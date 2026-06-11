// Package ui embeds the Tuck web dashboard and exposes it as an http.Handler.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets
var assets embed.FS

// Handler returns an http.Handler that serves the embedded dashboard files.
// Mount it under a path prefix and strip that prefix before passing to Handler.
//
// Example:
//
//	mux.Handle("/ui/", http.StripPrefix("/ui", ui.Handler()))
func Handler() http.Handler {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		// assets directory is always present (embedded at build time).
		panic("ui: sub FS: " + err.Error())
	}
	return http.FileServer(http.FS(sub))
}
