package api

import "net/http"

// wantsList reports whether the request is a Vault-compatible list operation
// using GET with ?list=true (required for browser clients — fetch cannot send LIST).
func wantsList(r *http.Request) bool {
	return r.URL.Query().Get("list") == "true"
}

// registerListCompat registers LIST and an equivalent GET ?list=true handler on path.
func (s *Server) registerListCompat(mux *http.ServeMux, path string, listFn http.HandlerFunc) {
	h := s.requireToken(listFn)
	mux.HandleFunc("LIST "+path, h)
	mux.HandleFunc("GET "+path, s.requireToken(func(w http.ResponseWriter, r *http.Request) {
		if !wantsList(r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "collection listing requires ?list=true",
			})
			return
		}
		listFn(w, r)
	}))
}
