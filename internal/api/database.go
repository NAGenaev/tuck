package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/dynamic/database"
)

// PUT /v1/database/config/{name}
// Body: {"plugin_name":"postgresql","connection_url":"postgres://...","max_open_conns":5}
func (s *Server) putDBConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config name required"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var cfg database.Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	cfg.Name = name
	if cfg.PluginName == "" || cfg.ConnectionURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plugin_name and connection_url required"})
		return
	}
	if err := s.core.PutDBConfig(r.Context(), &cfg); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, &cfg)
}

// GET /v1/database/config/{name}
func (s *Server) getDBConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg, err := s.core.GetDBConfig(r.Context(), name)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "config not found"})
			return
		}
		writeErr(w, err)
		return
	}
	// Mask the connection URL to avoid leaking credentials.
	cfg.ConnectionURL = "[redacted]"
	writeJSON(w, http.StatusOK, cfg)
}

// DELETE /v1/database/config/{name}
func (s *Server) deleteDBConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.DeleteDBConfig(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/database/config/
func (s *Server) listDBConfigs(w http.ResponseWriter, r *http.Request) {
	names, err := s.core.ListDBConfigs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}

// PUT /v1/database/role/{name}
// Body: {"db_name":"mydb","creation_statements":"...","revocation_statements":"...","default_ttl":"1h","max_ttl":"24h"}
func (s *Server) putDBRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role name required"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var wire struct {
		DBName               string `json:"db_name"`
		CreationStatements   string `json:"creation_statements"`
		RevocationStatements string `json:"revocation_statements"`
		DefaultTTL           string `json:"default_ttl"`
		MaxTTL               string `json:"max_ttl"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if wire.DBName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "db_name required"})
		return
	}

	role := &database.Role{
		Name:                 name,
		DBName:               wire.DBName,
		CreationStatements:   wire.CreationStatements,
		RevocationStatements: wire.RevocationStatements,
	}
	if wire.DefaultTTL != "" {
		d, err := time.ParseDuration(wire.DefaultTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid default_ttl: " + err.Error()})
			return
		}
		role.DefaultTTL = d
	}
	if wire.MaxTTL != "" {
		d, err := time.ParseDuration(wire.MaxTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid max_ttl: " + err.Error()})
			return
		}
		role.MaxTTL = d
	}

	if err := s.core.PutDBRole(r.Context(), role); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// GET /v1/database/role/{name}
func (s *Server) getDBRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	role, err := s.core.GetDBRole(r.Context(), name)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// DELETE /v1/database/role/{name}
func (s *Server) deleteDBRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.DeleteDBRole(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/database/role/
func (s *Server) listDBRoles(w http.ResponseWriter, r *http.Request) {
	names, err := s.core.ListDBRoles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}

// POST /v1/database/creds/{role}
func (s *Server) generateDBCreds(w http.ResponseWriter, r *http.Request) {
	role := r.PathValue("role")
	creds, err := s.core.GenerateDBCreds(r.Context(), role)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role or db config not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, creds)
}

// GET /v1/database/lease/{id}
func (s *Server) getDBLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lease, err := s.core.GetDBLease(r.Context(), id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lease not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lease)
}

// DELETE /v1/database/lease/{id}
func (s *Server) revokeDBLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.RevokeDBLease(r.Context(), id); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lease not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/database/lease/
func (s *Server) listDBLeases(w http.ResponseWriter, r *http.Request) {
	ids, err := s.core.ListDBLeases(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": ids})
}
