package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/NAGenaev/tuck/internal/identity"
)

// ── Entity handlers ──────────────────────────────────────────────────────────

// POST /v1/identity/entity
func (s *Server) identityCreateEntity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string            `json:"name"`
		Policies []string          `json:"policies"`
		Metadata map[string]string `json:"metadata"`
		Disabled bool              `json:"disabled"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	e, err := s.core.IdentityCreateEntity(r.Context(), req.Name, req.Policies, req.Metadata)
	if err != nil {
		writeErr(w, err)
		return
	}
	if req.Disabled {
		e.Disabled = true
		_ = s.core.IdentityPutEntity(r.Context(), e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": e.ID, "data": e})
}

// GET /v1/identity/entity/id/{id}
func (s *Server) identityGetEntityByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	e, err := s.core.IdentityGetEntityByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, identity.ErrEntityNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "entity not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": e.ID, "data": e})
}

// POST /v1/identity/entity/id/{id}  (update)
func (s *Server) identityUpdateEntityByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	e, err := s.core.IdentityGetEntityByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, identity.ErrEntityNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "entity not found"})
			return
		}
		writeErr(w, err)
		return
	}
	var req struct {
		Name     *string           `json:"name"`
		Policies []string          `json:"policies"`
		Metadata map[string]string `json:"metadata"`
		Disabled *bool             `json:"disabled"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Name != nil {
		e.Name = *req.Name
	}
	if req.Policies != nil {
		e.Policies = req.Policies
	}
	if req.Metadata != nil {
		e.Metadata = req.Metadata
	}
	if req.Disabled != nil {
		e.Disabled = *req.Disabled
	}
	if err := s.core.IdentityPutEntity(r.Context(), e); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": e.ID, "data": e})
}

// DELETE /v1/identity/entity/id/{id}
func (s *Server) identityDeleteEntityByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.IdentityDeleteEntity(r.Context(), id); err != nil {
		if errors.Is(err, identity.ErrEntityNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "entity not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/identity/entity/name/{name}
func (s *Server) identityGetEntityByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	e, err := s.core.IdentityGetEntityByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, identity.ErrEntityNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "entity not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": e.ID, "data": e})
}

// POST /v1/identity/entity/name/{name}  (upsert by name)
func (s *Server) identityUpsertEntityByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Policies []string          `json:"policies"`
		Metadata map[string]string `json:"metadata"`
		Disabled *bool             `json:"disabled"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	e, err := s.core.IdentityGetEntityByName(r.Context(), name)
	if errors.Is(err, identity.ErrEntityNotFound) {
		e, err = s.core.IdentityCreateEntity(r.Context(), name, req.Policies, req.Metadata)
		if err != nil {
			writeErr(w, err)
			return
		}
	} else if err != nil {
		writeErr(w, err)
		return
	} else {
		if req.Policies != nil {
			e.Policies = req.Policies
		}
		if req.Metadata != nil {
			e.Metadata = req.Metadata
		}
		if req.Disabled != nil {
			e.Disabled = *req.Disabled
		}
		if err := s.core.IdentityPutEntity(r.Context(), e); err != nil {
			writeErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": e.ID, "data": e})
}

// DELETE /v1/identity/entity/name/{name}
func (s *Server) identityDeleteEntityByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	e, err := s.core.IdentityGetEntityByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, identity.ErrEntityNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "entity not found"})
			return
		}
		writeErr(w, err)
		return
	}
	if err := s.core.IdentityDeleteEntity(r.Context(), e.ID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/identity/entity/
func (s *Server) identityListEntities(w http.ResponseWriter, r *http.Request) {
	ids, err := s.core.IdentityListEntityIDs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": ids})
}

// ── EntityAlias handlers ─────────────────────────────────────────────────────

// POST /v1/identity/entity-alias
func (s *Server) identityCreateAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EntityID      string            `json:"entity_id"`
		MountAccessor string            `json:"mount_accessor"`
		Name          string            `json:"name"`
		Metadata      map[string]string `json:"metadata"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.EntityID == "" || req.MountAccessor == "" || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "entity_id, mount_accessor, and name required"})
		return
	}
	a, err := s.core.IdentityCreateAlias(r.Context(), req.EntityID, req.MountAccessor, req.Name, req.Metadata)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": a.ID, "data": a})
}

// GET /v1/identity/entity-alias/id/{id}
func (s *Server) identityGetAlias(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := s.core.IdentityGetAliasByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, identity.ErrAliasNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "alias not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": a.ID, "data": a})
}

// POST /v1/identity/entity-alias/id/{id}  (update)
func (s *Server) identityUpdateAlias(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := s.core.IdentityGetAliasByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, identity.ErrAliasNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "alias not found"})
			return
		}
		writeErr(w, err)
		return
	}
	var req struct {
		Name     *string           `json:"name"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Name != nil {
		a.Name = *req.Name
	}
	if req.Metadata != nil {
		a.Metadata = req.Metadata
	}
	if err := s.core.IdentityPutAlias(r.Context(), a); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": a.ID, "data": a})
}

// DELETE /v1/identity/entity-alias/id/{id}
func (s *Server) identityDeleteAlias(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.IdentityDeleteAlias(r.Context(), id); err != nil {
		if errors.Is(err, identity.ErrAliasNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "alias not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/identity/entity-alias/id/
func (s *Server) identityListAliases(w http.ResponseWriter, r *http.Request) {
	ids, err := s.core.IdentityListAliasIDs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": ids})
}

// ── Group handlers ───────────────────────────────────────────────────────────

// POST /v1/identity/group
func (s *Server) identityCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string            `json:"name"`
		Policies        []string          `json:"policies"`
		MemberEntityIDs []string          `json:"member_entity_ids"`
		MemberGroupIDs  []string          `json:"member_group_ids"`
		Metadata        map[string]string `json:"metadata"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	g, err := s.core.IdentityCreateGroup(r.Context(), req.Name, req.Policies, req.MemberEntityIDs, req.MemberGroupIDs, req.Metadata)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": g.ID, "data": g})
}

// GET /v1/identity/group/id/{id}
func (s *Server) identityGetGroupByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	g, err := s.core.IdentityGetGroupByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, identity.ErrGroupNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": g.ID, "data": g})
}

// POST /v1/identity/group/id/{id}  (update)
func (s *Server) identityUpdateGroupByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	g, err := s.core.IdentityGetGroupByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, identity.ErrGroupNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
			return
		}
		writeErr(w, err)
		return
	}
	var req struct {
		Name            *string           `json:"name"`
		Policies        []string          `json:"policies"`
		MemberEntityIDs []string          `json:"member_entity_ids"`
		MemberGroupIDs  []string          `json:"member_group_ids"`
		Metadata        map[string]string `json:"metadata"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Name != nil {
		g.Name = *req.Name
	}
	if req.Policies != nil {
		g.Policies = req.Policies
	}
	if req.MemberEntityIDs != nil {
		g.MemberEntityIDs = req.MemberEntityIDs
	}
	if req.MemberGroupIDs != nil {
		g.MemberGroupIDs = req.MemberGroupIDs
	}
	if req.Metadata != nil {
		g.Metadata = req.Metadata
	}
	if err := s.core.IdentityPutGroup(r.Context(), g); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": g.ID, "data": g})
}

// DELETE /v1/identity/group/id/{id}
func (s *Server) identityDeleteGroupByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.IdentityDeleteGroup(r.Context(), id); err != nil {
		if errors.Is(err, identity.ErrGroupNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/identity/group/name/{name}
func (s *Server) identityGetGroupByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	g, err := s.core.IdentityGetGroupByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, identity.ErrGroupNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": g.ID, "data": g})
}

// POST /v1/identity/group/name/{name}  (upsert by name)
func (s *Server) identityUpsertGroupByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Policies        []string          `json:"policies"`
		MemberEntityIDs []string          `json:"member_entity_ids"`
		MemberGroupIDs  []string          `json:"member_group_ids"`
		Metadata        map[string]string `json:"metadata"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	g, err := s.core.IdentityGetGroupByName(r.Context(), name)
	if errors.Is(err, identity.ErrGroupNotFound) {
		g, err = s.core.IdentityCreateGroup(r.Context(), name, req.Policies, req.MemberEntityIDs, req.MemberGroupIDs, req.Metadata)
		if err != nil {
			writeErr(w, err)
			return
		}
	} else if err != nil {
		writeErr(w, err)
		return
	} else {
		if req.Policies != nil {
			g.Policies = req.Policies
		}
		if req.MemberEntityIDs != nil {
			g.MemberEntityIDs = req.MemberEntityIDs
		}
		if req.MemberGroupIDs != nil {
			g.MemberGroupIDs = req.MemberGroupIDs
		}
		if req.Metadata != nil {
			g.Metadata = req.Metadata
		}
		if err := s.core.IdentityPutGroup(r.Context(), g); err != nil {
			writeErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": g.ID, "data": g})
}

// DELETE /v1/identity/group/name/{name}
func (s *Server) identityDeleteGroupByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	g, err := s.core.IdentityGetGroupByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, identity.ErrGroupNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
			return
		}
		writeErr(w, err)
		return
	}
	if err := s.core.IdentityDeleteGroup(r.Context(), g.ID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/identity/group/
func (s *Server) identityListGroups(w http.ResponseWriter, r *http.Request) {
	ids, err := s.core.IdentityListGroupIDs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": ids})
}

// ── Lookup handlers ──────────────────────────────────────────────────────────

// POST /v1/identity/lookup/entity
// Body: {"id":"..."} or {"name":"..."} or {"alias_name":"...","alias_mount_accessor":"..."}
func (s *Server) identityLookupEntity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID                 string `json:"id"`
		Name               string `json:"name"`
		AliasName          string `json:"alias_name"`
		AliasMountAccessor string `json:"alias_mount_accessor"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var (
		e   *identity.Entity
		err error
	)
	switch {
	case req.ID != "":
		e, err = s.core.IdentityGetEntityByID(r.Context(), req.ID)
	case req.Name != "":
		e, err = s.core.IdentityGetEntityByName(r.Context(), req.Name)
	case req.AliasName != "" && req.AliasMountAccessor != "":
		var a *identity.EntityAlias
		a, err = s.core.IdentityGetAliasByMount(r.Context(), req.AliasMountAccessor, req.AliasName)
		if err == nil {
			e, err = s.core.IdentityGetEntityByID(r.Context(), a.EntityID)
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "one of id, name, or alias_name+alias_mount_accessor required"})
		return
	}
	if err != nil {
		if errors.Is(err, identity.ErrEntityNotFound) || errors.Is(err, identity.ErrAliasNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "entity not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": e.ID, "data": e})
}

// POST /v1/identity/lookup/group
// Body: {"id":"..."} or {"name":"..."}
func (s *Server) identityLookupGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var (
		g   *identity.Group
		err error
	)
	switch {
	case req.ID != "":
		g, err = s.core.IdentityGetGroupByID(r.Context(), req.ID)
	case req.Name != "":
		g, err = s.core.IdentityGetGroupByName(r.Context(), req.Name)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id or name required"})
		return
	}
	if err != nil {
		if errors.Is(err, identity.ErrGroupNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": g.ID, "data": g})
}

// ── Group-alias handlers ─────────────────────────────────────────────────────

// POST /v1/identity/group-alias
func (s *Server) identityCreateGroupAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GroupID       string            `json:"group_id"`
		MountAccessor string            `json:"mount_accessor"`
		Name          string            `json:"name"`
		Metadata      map[string]string `json:"metadata"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.GroupID == "" || req.MountAccessor == "" || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "group_id, mount_accessor and name required"})
		return
	}
	ga, err := s.core.IdentityCreateGroupAlias(r.Context(), req.GroupID, req.MountAccessor, req.Name, req.Metadata)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": ga.ID, "data": ga})
}

// GET /v1/identity/group-alias/id/:id
func (s *Server) identityGetGroupAliasByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	ga, err := s.core.IdentityGetGroupAliasByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, identity.ErrGroupAliasNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "group alias not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": ga.ID, "data": ga})
}

// DELETE /v1/identity/group-alias/id/:id
func (s *Server) identityDeleteGroupAlias(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	if err := s.core.IdentityDeleteGroupAlias(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// GET /v1/identity/group-alias?list=true
func (s *Server) identityListGroupAliases(w http.ResponseWriter, r *http.Request) {
	ids, err := s.core.IdentityListGroupAliasIDs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": ids})
}

// ── helpers ──────────────────────────────────────────────────────────────────

func readJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}
