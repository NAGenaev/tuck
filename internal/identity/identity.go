// Package identity implements the Entity & Identity system.
//
// Entities represent canonical identities (persons or services) that can
// have aliases from multiple auth methods, direct policy attachments, and
// group memberships. Tokens carry an EntityID so their effective policy set
// is the union of token policies + entity policies + group policies.
package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

// Storage key layout (all under the barrier):
//   identity/entity/id/<id>                    → Entity JSON
//   identity/entity/name/<name>                → entity id (pointer)
//   identity/alias/id/<id>                    → EntityAlias JSON
//   identity/alias/mount/<mount>/<name>       → alias id (index)
//   identity/group/id/<id>                    → Group JSON
//   identity/group/name/<name>                → group id (pointer)
//   identity/group-alias/id/<id>              → GroupAlias JSON
//   identity/group-alias/mount/<mount>/<name> → group-alias id (index)

const (
	entityIDPrefix    = "identity/entity/id/"
	entityNamePrefix  = "identity/entity/name/"
	aliasIDPrefix     = "identity/alias/id/"
	aliasMountPrefix  = "identity/alias/mount/"
	groupIDPrefix     = "identity/group/id/"
	groupNamePrefix   = "identity/group/name/"
	gAliasIDPrefix    = "identity/group-alias/id/"
	gAliasMountPrefix = "identity/group-alias/mount/"
)

var (
	ErrEntityNotFound     = errors.New("identity: entity not found")
	ErrGroupNotFound      = errors.New("identity: group not found")
	ErrAliasNotFound      = errors.New("identity: entity alias not found")
	ErrGroupAliasNotFound = errors.New("identity: group alias not found")
)

// Entity is a canonical identity that can be linked to multiple auth method
// logins via EntityAliases.
type Entity struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Policies []string          `json:"policies,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Disabled bool              `json:"disabled,omitempty"`
	Created  time.Time         `json:"created_at"`
	Updated  time.Time         `json:"updated_at"`
}

// EntityAlias links one auth-method identity (e.g. LDAP username, AppRole
// role name, JWT subject) to an Entity.
type EntityAlias struct {
	ID            string            `json:"id"`
	EntityID      string            `json:"entity_id"`
	MountAccessor string            `json:"mount_accessor"`
	Name          string            `json:"name"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Created       time.Time         `json:"created_at"`
	Updated       time.Time         `json:"updated_at"`
}

// GroupAlias links an external group identity (e.g. an LDAP group DN, a JWT
// "groups" claim value) to a Tuck Group. When a user logs in and their
// external groups are resolved, Tuck looks up matching GroupAliases and adds
// those groups' policies to the issued token.
type GroupAlias struct {
	ID            string            `json:"id"`
	GroupID       string            `json:"group_id"`
	MountAccessor string            `json:"mount_accessor"`
	Name          string            `json:"name"` // external group name, e.g. LDAP DN
	Metadata      map[string]string `json:"metadata,omitempty"`
	Created       time.Time         `json:"created_at"`
	Updated       time.Time         `json:"updated_at"`
}

// Group is a collection of entities. Its policies apply to all members.
// Nested groups are supported via MemberGroupIDs: members of a child group
// are also considered members of the parent group.
type Group struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Policies        []string          `json:"policies,omitempty"`
	MemberEntityIDs []string          `json:"member_entity_ids,omitempty"`
	MemberGroupIDs  []string          `json:"member_group_ids,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Created         time.Time         `json:"created_at"`
	Updated         time.Time         `json:"updated_at"`
}

type barrierIface interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Store persists entities, aliases, and groups inside the encrypted barrier.
type Store struct{ b barrierIface }

// NewStore creates a Store backed by b.
func NewStore(b barrierIface) *Store { return &Store{b: b} }

func newID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// ── Entity CRUD ─────────────────────────────────────────────────────────────

// CreateEntity generates a new entity with a random ID.
func (s *Store) CreateEntity(ctx context.Context, name string, policies []string, meta map[string]string) (*Entity, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	e := &Entity{ID: id, Name: name, Policies: policies, Metadata: meta, Created: now, Updated: now}
	return e, s.PutEntity(ctx, e)
}

// PutEntity stores (create or update) an entity.
func (s *Store) PutEntity(ctx context.Context, e *Entity) error {
	e.Updated = time.Now().UTC()
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := s.b.Put(ctx, &physical.Entry{Key: entityIDPrefix + e.ID, Value: data}); err != nil {
		return err
	}
	return s.b.Put(ctx, &physical.Entry{Key: entityNamePrefix + e.Name, Value: []byte(e.ID)})
}

// GetEntityByID returns the entity with the given ID.
func (s *Store) GetEntityByID(ctx context.Context, id string) (*Entity, error) {
	e, err := s.b.Get(ctx, entityIDPrefix+id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrEntityNotFound
	}
	var entity Entity
	return &entity, json.Unmarshal(e.Value, &entity)
}

// GetEntityByName returns the entity with the given name.
func (s *Store) GetEntityByName(ctx context.Context, name string) (*Entity, error) {
	e, err := s.b.Get(ctx, entityNamePrefix+name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrEntityNotFound
	}
	return s.GetEntityByID(ctx, string(e.Value))
}

// DeleteEntity removes the entity and its name index entry.
func (s *Store) DeleteEntity(ctx context.Context, id string) error {
	entity, err := s.GetEntityByID(ctx, id)
	if err != nil {
		return err
	}
	_ = s.b.Delete(ctx, entityNamePrefix+entity.Name)
	return s.b.Delete(ctx, entityIDPrefix+id)
}

// ListEntityIDs returns all entity IDs.
func (s *Store) ListEntityIDs(ctx context.Context) ([]string, error) {
	keys, err := s.b.List(ctx, entityIDPrefix)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = strings.TrimPrefix(k, entityIDPrefix)
	}
	return ids, nil
}

// ── EntityAlias CRUD ─────────────────────────────────────────────────────────

// CreateAlias creates a new alias linking (mount, name) to entityID.
func (s *Store) CreateAlias(ctx context.Context, entityID, mount, name string, meta map[string]string) (*EntityAlias, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	a := &EntityAlias{ID: id, EntityID: entityID, MountAccessor: mount, Name: name, Metadata: meta, Created: now, Updated: now}
	return a, s.PutAlias(ctx, a)
}

// PutAlias stores (create or update) an entity alias.
func (s *Store) PutAlias(ctx context.Context, a *EntityAlias) error {
	a.Updated = time.Now().UTC()
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if err := s.b.Put(ctx, &physical.Entry{Key: aliasIDPrefix + a.ID, Value: data}); err != nil {
		return err
	}
	return s.b.Put(ctx, &physical.Entry{Key: aliasMountPrefix + a.MountAccessor + "/" + a.Name, Value: []byte(a.ID)})
}

// GetAliasByID returns the alias with the given ID.
func (s *Store) GetAliasByID(ctx context.Context, id string) (*EntityAlias, error) {
	e, err := s.b.Get(ctx, aliasIDPrefix+id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrAliasNotFound
	}
	var a EntityAlias
	return &a, json.Unmarshal(e.Value, &a)
}

// GetAliasByMount looks up an alias by auth-method accessor + alias name.
func (s *Store) GetAliasByMount(ctx context.Context, mount, name string) (*EntityAlias, error) {
	e, err := s.b.Get(ctx, aliasMountPrefix+mount+"/"+name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrAliasNotFound
	}
	return s.GetAliasByID(ctx, string(e.Value))
}

// DeleteAlias removes an alias by ID.
func (s *Store) DeleteAlias(ctx context.Context, id string) error {
	a, err := s.GetAliasByID(ctx, id)
	if err != nil {
		return err
	}
	_ = s.b.Delete(ctx, aliasMountPrefix+a.MountAccessor+"/"+a.Name)
	return s.b.Delete(ctx, aliasIDPrefix+id)
}

// ListAliasIDs returns all alias IDs.
func (s *Store) ListAliasIDs(ctx context.Context) ([]string, error) {
	keys, err := s.b.List(ctx, aliasIDPrefix)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = strings.TrimPrefix(k, aliasIDPrefix)
	}
	return ids, nil
}

// EnsureAlias looks up the alias for (mount, name). If it does not exist, a
// new Entity and EntityAlias are auto-created. Returns the Entity.
// This is called on every successful auth-method login.
func (s *Store) EnsureAlias(ctx context.Context, mount, name string) (*Entity, error) {
	// Fast path: alias already exists.
	alias, err := s.GetAliasByMount(ctx, mount, name)
	if err == nil {
		return s.GetEntityByID(ctx, alias.EntityID)
	}

	// Auto-create entity. Use mount:name as the display name; append a
	// short suffix if that name is already taken.
	autoName := mount + ":" + name
	if _, err := s.GetEntityByName(ctx, autoName); err == nil {
		suffix, _ := newID()
		autoName = autoName + "-" + suffix[:6]
	}

	entity, err := s.CreateEntity(ctx, autoName, nil, nil)
	if err != nil {
		return nil, err
	}
	if _, err := s.CreateAlias(ctx, entity.ID, mount, name, nil); err != nil {
		return nil, err
	}
	return entity, nil
}

// ── Group CRUD ───────────────────────────────────────────────────────────────

// CreateGroup creates a new group with a random ID.
func (s *Store) CreateGroup(ctx context.Context, name string, policies []string, memberEntityIDs, memberGroupIDs []string, meta map[string]string) (*Group, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	g := &Group{
		ID: id, Name: name, Policies: policies,
		MemberEntityIDs: memberEntityIDs, MemberGroupIDs: memberGroupIDs,
		Metadata: meta, Created: now, Updated: now,
	}
	return g, s.PutGroup(ctx, g)
}

// PutGroup stores (create or update) a group.
func (s *Store) PutGroup(ctx context.Context, g *Group) error {
	g.Updated = time.Now().UTC()
	data, err := json.Marshal(g)
	if err != nil {
		return err
	}
	if err := s.b.Put(ctx, &physical.Entry{Key: groupIDPrefix + g.ID, Value: data}); err != nil {
		return err
	}
	return s.b.Put(ctx, &physical.Entry{Key: groupNamePrefix + g.Name, Value: []byte(g.ID)})
}

// GetGroupByID returns the group with the given ID.
func (s *Store) GetGroupByID(ctx context.Context, id string) (*Group, error) {
	e, err := s.b.Get(ctx, groupIDPrefix+id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrGroupNotFound
	}
	var g Group
	return &g, json.Unmarshal(e.Value, &g)
}

// GetGroupByName returns the group with the given name.
func (s *Store) GetGroupByName(ctx context.Context, name string) (*Group, error) {
	e, err := s.b.Get(ctx, groupNamePrefix+name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrGroupNotFound
	}
	return s.GetGroupByID(ctx, string(e.Value))
}

// DeleteGroup removes the group and its name index entry.
func (s *Store) DeleteGroup(ctx context.Context, id string) error {
	g, err := s.GetGroupByID(ctx, id)
	if err != nil {
		return err
	}
	_ = s.b.Delete(ctx, groupNamePrefix+g.Name)
	return s.b.Delete(ctx, groupIDPrefix+id)
}

// ListGroupIDs returns all group IDs.
func (s *Store) ListGroupIDs(ctx context.Context) ([]string, error) {
	keys, err := s.b.List(ctx, groupIDPrefix)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = strings.TrimPrefix(k, groupIDPrefix)
	}
	return ids, nil
}

// ── GroupAlias CRUD ──────────────────────────────────────────────────────────

// CreateGroupAlias creates a new alias linking (mount, name) to groupID.
func (s *Store) CreateGroupAlias(ctx context.Context, groupID, mount, name string, meta map[string]string) (*GroupAlias, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	a := &GroupAlias{ID: id, GroupID: groupID, MountAccessor: mount, Name: name, Metadata: meta, Created: now, Updated: now}
	return a, s.PutGroupAlias(ctx, a)
}

// PutGroupAlias stores (create or update) a group alias.
func (s *Store) PutGroupAlias(ctx context.Context, a *GroupAlias) error {
	a.Updated = time.Now().UTC()
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if err := s.b.Put(ctx, &physical.Entry{Key: gAliasIDPrefix + a.ID, Value: data}); err != nil {
		return err
	}
	return s.b.Put(ctx, &physical.Entry{Key: gAliasMountPrefix + a.MountAccessor + "/" + a.Name, Value: []byte(a.ID)})
}

// GetGroupAliasByID returns the group alias with the given ID.
func (s *Store) GetGroupAliasByID(ctx context.Context, id string) (*GroupAlias, error) {
	e, err := s.b.Get(ctx, gAliasIDPrefix+id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrGroupAliasNotFound
	}
	var a GroupAlias
	return &a, json.Unmarshal(e.Value, &a)
}

// GetGroupAliasByMount looks up a group alias by mount accessor + external name.
func (s *Store) GetGroupAliasByMount(ctx context.Context, mount, name string) (*GroupAlias, error) {
	e, err := s.b.Get(ctx, gAliasMountPrefix+mount+"/"+name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrGroupAliasNotFound
	}
	return s.GetGroupAliasByID(ctx, string(e.Value))
}

// DeleteGroupAlias removes a group alias by ID.
func (s *Store) DeleteGroupAlias(ctx context.Context, id string) error {
	a, err := s.GetGroupAliasByID(ctx, id)
	if err != nil {
		return err
	}
	_ = s.b.Delete(ctx, gAliasMountPrefix+a.MountAccessor+"/"+a.Name)
	return s.b.Delete(ctx, gAliasIDPrefix+id)
}

// ListGroupAliasIDs returns all group alias IDs.
func (s *Store) ListGroupAliasIDs(ctx context.Context) ([]string, error) {
	keys, err := s.b.List(ctx, gAliasIDPrefix)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = strings.TrimPrefix(k, gAliasIDPrefix)
	}
	return ids, nil
}

// ── Policy resolution ────────────────────────────────────────────────────────

// ResolveEntityPolicies returns the union of policies that apply to an entity:
//   - entity's own policies
//   - policies of every group the entity is a direct or transitive member of
//     (A group G₂ grants membership to all members of G₁ when G₁.ID ∈ G₂.MemberGroupIDs)
func (s *Store) ResolveEntityPolicies(ctx context.Context, entityID string) []string {
	entity, err := s.GetEntityByID(ctx, entityID)
	if err != nil || entity.Disabled {
		return nil
	}

	seen := make(map[string]bool)
	var result []string
	add := func(policies []string) {
		for _, p := range policies {
			if !seen[p] {
				seen[p] = true
				result = append(result, p)
			}
		}
	}
	add(entity.Policies)

	// Load all groups once.
	groupIDs, err := s.ListGroupIDs(ctx)
	if err != nil {
		return result
	}
	allGroups := make([]*Group, 0, len(groupIDs))
	for _, gid := range groupIDs {
		if g, err := s.GetGroupByID(ctx, gid); err == nil {
			allGroups = append(allGroups, g)
		}
	}

	// BFS: start from groups where entity is a direct member, then expand
	// upward to parent groups (groups whose MemberGroupIDs include a found group).
	memberSet := make(map[string]bool)
	queue := make([]string, 0)
	for _, g := range allGroups {
		for _, eid := range g.MemberEntityIDs {
			if eid == entityID {
				queue = append(queue, g.ID)
				break
			}
		}
	}
	for len(queue) > 0 {
		gid := queue[0]
		queue = queue[1:]
		if memberSet[gid] {
			continue
		}
		memberSet[gid] = true
		// Find parent groups that include this group as a sub-group.
		for _, pg := range allGroups {
			if memberSet[pg.ID] {
				continue
			}
			for _, childID := range pg.MemberGroupIDs {
				if childID == gid {
					queue = append(queue, pg.ID)
					break
				}
			}
		}
	}

	for gid := range memberSet {
		for _, g := range allGroups {
			if g.ID == gid {
				add(g.Policies)
				break
			}
		}
	}
	return result
}

// ResolveExternalGroupPolicies returns policies from Tuck groups that are
// aliased to any of the given external group names under the given mount
// accessor (e.g. "auth_ldap", "auth_jwt"). It also follows nested group
// memberships upward via the same BFS used by ResolveEntityPolicies.
func (s *Store) ResolveExternalGroupPolicies(ctx context.Context, mount string, externalGroups []string) []string {
	if len(externalGroups) == 0 {
		return nil
	}

	// Collect Tuck group IDs that are aliased to any external group.
	matchedGroupIDs := make(map[string]bool)
	for _, extGroup := range externalGroups {
		ga, err := s.GetGroupAliasByMount(ctx, mount, extGroup)
		if err == nil {
			matchedGroupIDs[ga.GroupID] = true
		}
	}
	if len(matchedGroupIDs) == 0 {
		return nil
	}

	// Load all groups once for BFS.
	groupIDs, err := s.ListGroupIDs(ctx)
	if err != nil {
		return nil
	}
	allGroups := make([]*Group, 0, len(groupIDs))
	for _, gid := range groupIDs {
		if g, err := s.GetGroupByID(ctx, gid); err == nil {
			allGroups = append(allGroups, g)
		}
	}

	// BFS upward: expand matched groups to include their parent groups.
	queue := make([]string, 0, len(matchedGroupIDs))
	for gid := range matchedGroupIDs {
		queue = append(queue, gid)
	}
	visited := make(map[string]bool)
	for len(queue) > 0 {
		gid := queue[0]
		queue = queue[1:]
		if visited[gid] {
			continue
		}
		visited[gid] = true
		for _, pg := range allGroups {
			if visited[pg.ID] {
				continue
			}
			for _, childID := range pg.MemberGroupIDs {
				if childID == gid {
					queue = append(queue, pg.ID)
					break
				}
			}
		}
	}

	seen := make(map[string]bool)
	var result []string
	for gid := range visited {
		for _, g := range allGroups {
			if g.ID == gid {
				for _, p := range g.Policies {
					if !seen[p] {
						seen[p] = true
						result = append(result, p)
					}
				}
				break
			}
		}
	}
	return result
}
