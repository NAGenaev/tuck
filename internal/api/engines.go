package api

import (
	"net/http"
	"strings"
)

// ExperimentalEngines are the secret engines that sit outside Tuck's core
// scope (KV, Transit, Database, auth, audit). A production binary keeps them
// off unless an operator opts in via RestrictEngines.
var ExperimentalEngines = []string{"pki", "ssh", "totp", "aws", "gcp", "azure"}

// RestrictEngines turns on the experimental-engine gate: requests to the
// listed experimental engines are served, requests to every other
// experimental engine get 404. Without a call, the gate is off and all
// engines are reachable.
func (s *Server) RestrictEngines(enabled []string) {
	s.engines = make(map[string]bool, len(enabled))
	for _, e := range enabled {
		s.engines[strings.ToLower(strings.TrimSpace(e))] = true
	}
}

func (s *Server) engineGate(next http.Handler) http.Handler {
	if s.engines == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.Path, "/v1/")
		if ok {
			name, _, _ := strings.Cut(rest, "/")
			for _, e := range ExperimentalEngines {
				if name == e && !s.engines[e] {
					writeJSON(w, http.StatusNotFound, map[string]string{
						"error": "engine \"" + e + "\" is disabled (experimental); start the server with --enable-engine=" + e,
					})
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
