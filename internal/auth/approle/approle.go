// Package approle implements machine-to-machine authentication using
// role-id + secret-id pairs — no OIDC provider or Kubernetes dependency required.
//
// Flow:
//  1. Operator creates a Role (PUT /v1/auth/approle/role/{name}).
//  2. Operator generates a SecretID for that role (POST /v1/auth/approle/role/{name}/secret-id).
//  3. Application calls POST /v1/auth/approle/login with role_id + secret_id.
//  4. Tuck validates both, increments use-count, and returns a short-lived token.
package approle

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

var (
	// ErrNotFound is returned when a role or secret-id does not exist.
	ErrNotFound = errors.New("approle: not found")
	// ErrInvalidCredentials is returned when role_id or secret_id is wrong/expired/exhausted.
	ErrInvalidCredentials = errors.New("approle: invalid or expired credentials")
)

const (
	rolesPrefix     = "auth/approle/roles/"        // #nosec G101 — storage path prefix, not a credential
	secretIDsPrefix = "auth/approle/secret-ids/"   // #nosec G101 — storage path prefix, not a credential
)

// Role defines an AppRole: its role-id (public identifier), policies,
// and constraints on the generated SecretIDs.
type Role struct {
	Name string `json:"name"`
	// RoleID is the public, non-secret identifier for this role.
	// Auto-generated if empty.
	RoleID string `json:"role_id"`
	// Policies assigned to tokens issued via this role.
	Policies []string `json:"policies"`
	// SecretIDTTL is how long a generated SecretID remains valid.
	// Zero means SecretIDs never expire.
	SecretIDTTL time.Duration `json:"secret_id_ttl,omitempty"`
	// SecretIDNumUses is the maximum number of times a SecretID can be used.
	// Zero means unlimited.
	SecretIDNumUses int `json:"secret_id_num_uses,omitempty"`
	// TokenTTL is the lifetime of tokens issued via this role.
	TokenTTL time.Duration `json:"token_ttl,omitempty"`
	// BoundCIDRs restricts login to the listed CIDRs. Empty = no restriction.
	BoundCIDRs []string `json:"bound_cidrs,omitempty"`
}

// SecretID is a single-use (or limited-use) credential attached to a Role.
type SecretID struct {
	ID        string            `json:"id"`
	RoleName  string            `json:"role_name"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at,omitempty"` // zero = never
	NumUses   int               `json:"num_uses,omitempty"`   // 0 = unlimited
	UsesLeft  int               `json:"uses_left,omitempty"`  // 0 = unlimited
	// BoundCIDRs overrides the role-level CIDRs for this specific secret-id.
	BoundCIDRs []string         `json:"bound_cidrs,omitempty"`
	// Metadata is caller-supplied key/value annotations stored with the secret-id.
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// LoginResult is returned by Store.Login on success.
type LoginResult struct {
	Subject  string
	Policies []string
	TTL      time.Duration
}

// SecretIDOptions holds optional parameters for GenerateSecretIDWithOptions.
type SecretIDOptions struct {
	BoundCIDRs []string
	Metadata   map[string]string
}

type barrier interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Store manages AppRole roles and secret-IDs inside the encrypted barrier.
type Store struct{ b barrier }

// NewStore creates a Store backed by b.
func NewStore(b barrier) *Store { return &Store{b: b} }

// PutRole creates or replaces a role. Auto-generates RoleID if empty.
func (s *Store) PutRole(ctx context.Context, r *Role) error {
	if r.RoleID == "" {
		id, err := generateID()
		if err != nil {
			return fmt.Errorf("approle: generate role-id: %w", err)
		}
		r.RoleID = id
	}
	return s.put(ctx, rolesPrefix+r.Name, r)
}

// GetRole fetches a role by name.
func (s *Store) GetRole(ctx context.Context, name string) (*Role, error) {
	var r Role
	if err := s.get(ctx, rolesPrefix+name, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// DeleteRole removes a role. Does NOT remove associated secret-IDs.
func (s *Store) DeleteRole(ctx context.Context, name string) error {
	return s.b.Delete(ctx, rolesPrefix+name)
}

// ListRoles returns all role names.
func (s *Store) ListRoles(ctx context.Context) ([]string, error) {
	keys, err := s.b.List(ctx, rolesPrefix)
	if err != nil {
		return nil, err
	}
	for i, k := range keys {
		keys[i] = strings.TrimPrefix(k, rolesPrefix)
	}
	return keys, nil
}

// GenerateSecretID creates a new SecretID for the named role.
func (s *Store) GenerateSecretID(ctx context.Context, roleName string) (*SecretID, error) {
	return s.GenerateSecretIDWithOptions(ctx, roleName, SecretIDOptions{})
}

// GenerateSecretIDWithOptions creates a SecretID with optional CIDR restrictions
// and caller-supplied metadata.
func (s *Store) GenerateSecretIDWithOptions(ctx context.Context, roleName string, opts SecretIDOptions) (*SecretID, error) {
	role, err := s.GetRole(ctx, roleName)
	if err != nil {
		return nil, err
	}
	if err := validateCIDRs(opts.BoundCIDRs); err != nil {
		return nil, fmt.Errorf("approle: bound_cidrs: %w", err)
	}
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("approle: generate secret-id: %w", err)
	}
	sid := &SecretID{
		ID:         id,
		RoleName:   roleName,
		CreatedAt:  time.Now().UTC(),
		NumUses:    role.SecretIDNumUses,
		UsesLeft:   role.SecretIDNumUses,
		BoundCIDRs: opts.BoundCIDRs,
		Metadata:   opts.Metadata,
	}
	if role.SecretIDTTL > 0 {
		sid.ExpiresAt = sid.CreatedAt.Add(role.SecretIDTTL)
	}
	return sid, s.put(ctx, secretIDsPrefix+id, sid)
}

// Login validates roleID + secretID from the given remoteIP and returns the login result.
// remoteIP may be empty to skip CIDR validation (e.g. in tests).
// It increments the use-count and marks exhausted SecretIDs for deletion.
func (s *Store) Login(ctx context.Context, roleID, secretIDVal, remoteIP string) (*LoginResult, error) {
	// Find the role whose RoleID matches.
	names, err := s.b.List(ctx, rolesPrefix)
	if err != nil {
		return nil, err
	}
	var role *Role
	for _, k := range names {
		var r Role
		if err := s.get(ctx, k, &r); err != nil {
			continue
		}
		if r.RoleID == roleID {
			role = &r
			break
		}
	}
	if role == nil {
		return nil, ErrInvalidCredentials
	}

	// Validate the secret-id.
	var sid SecretID
	if err := s.get(ctx, secretIDsPrefix+secretIDVal, &sid); err != nil {
		return nil, ErrInvalidCredentials
	}
	if sid.RoleName != role.Name {
		return nil, ErrInvalidCredentials
	}
	if !sid.ExpiresAt.IsZero() && time.Now().After(sid.ExpiresAt) {
		_ = s.b.Delete(ctx, secretIDsPrefix+secretIDVal)
		return nil, ErrInvalidCredentials
	}

	// CIDR check: secret-id-level overrides role-level.
	cidrs := sid.BoundCIDRs
	if len(cidrs) == 0 {
		cidrs = role.BoundCIDRs
	}
	if len(cidrs) > 0 && remoteIP != "" {
		if !cidrContains(cidrs, remoteIP) {
			return nil, ErrInvalidCredentials
		}
	}

	// Decrement use count.
	if sid.NumUses > 0 {
		sid.UsesLeft--
		if sid.UsesLeft <= 0 {
			_ = s.b.Delete(ctx, secretIDsPrefix+secretIDVal)
		} else {
			_ = s.put(ctx, secretIDsPrefix+secretIDVal, &sid)
		}
	}

	ttl := role.TokenTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &LoginResult{
		Subject:  "approle:" + role.Name,
		Policies: role.Policies,
		TTL:      ttl,
	}, nil
}

// LookupSecretID returns a SecretID by its value (for introspection).
func (s *Store) LookupSecretID(ctx context.Context, id string) (*SecretID, error) {
	var sid SecretID
	if err := s.get(ctx, secretIDsPrefix+id, &sid); err != nil {
		return nil, err
	}
	return &sid, nil
}

// DestroySecretID removes a specific SecretID.
func (s *Store) DestroySecretID(ctx context.Context, id string) error {
	return s.b.Delete(ctx, secretIDsPrefix+id)
}

// --- helpers ---

func (s *Store) get(ctx context.Context, key string, dst interface{}) error {
	e, err := s.b.Get(ctx, key)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNotFound
	}
	return json.Unmarshal(e.Value, dst)
}

func (s *Store) put(ctx context.Context, key string, src interface{}) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return s.b.Put(ctx, &physical.Entry{Key: key, Value: data})
}

func generateID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// validateCIDRs returns an error if any of the CIDR strings is not valid.
func validateCIDRs(cidrs []string) error {
	for _, c := range cidrs {
		if _, _, err := net.ParseCIDR(c); err != nil {
			return fmt.Errorf("invalid CIDR %q: %w", c, err)
		}
	}
	return nil
}

// cidrContains reports whether ip falls inside any of the given CIDRs.
func cidrContains(cidrs []string, ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, c := range cidrs {
		_, ipNet, err := net.ParseCIDR(c)
		if err != nil {
			continue
		}
		if ipNet.Contains(parsed) {
			return true
		}
	}
	return false
}
