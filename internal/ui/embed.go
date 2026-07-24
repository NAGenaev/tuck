// Package ui embeds Tuck's web dashboard (a React SPA forked from
// Remnawave's frontend shell, AGPL-3.0-only — see web/NOTICE) and exposes it
// as an http.Handler.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed assets
var assets embed.FS

// Handler returns an http.Handler that serves the embedded SPA build,
// falling back to index.html for any path that isn't a real embedded file
// so client-side (history-mode) routes survive a page reload.
func Handler() http.Handler {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		// assets directory is always present (embedded at build time).
		panic("ui: sub FS: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(sub, strings.TrimPrefix(r.URL.Path, "/")); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		r2 := new(http.Request)
		*r2 = *r
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
