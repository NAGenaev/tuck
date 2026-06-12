// Package ldap implements LDAP / Active Directory authentication for Tuck.
//
// Login flow:
//  1. Connect to the configured LDAP server(s).
//  2. Bind with the service account (BindDN + BindPassword) to search.
//  3. Search for the user entry by username attribute.
//  4. Bind as the user to verify the supplied password.
//  5. Collect the user's group membership (memberOf or explicit group search).
//  6. Match groups / username against configured Roles → union of policies.
package ldap

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	goldap "github.com/go-ldap/ldap/v3"

	"github.com/NAGenaev/tuck/internal/physical"
)

var (
	// ErrNotConfigured is returned when LDAP auth has not been set up yet.
	ErrNotConfigured = errors.New("ldap: auth not configured — PUT /v1/auth/ldap/config first")
	// ErrInvalidCredentials is returned when the username or password is wrong.
	ErrInvalidCredentials = errors.New("ldap: invalid username or password")
	// ErrNoRole is returned when no role matches the authenticated user.
	ErrNoRole = errors.New("ldap: no matching role for user")
)

const (
	configKey = "auth/ldap/config"
	rolesKey  = "auth/ldap/roles/"
)

// Config holds the LDAP connection and search parameters.
// Stored encrypted in the barrier; BindPassword is never returned by the API.
type Config struct {
	// URLs are LDAP server addresses in preference order.
	// Use ldap:// for plain or STARTTLS, ldaps:// for TLS.
	URLs []string `json:"urls"`

	// BindDN is the service account used for user and group searches.
	// Example: "cn=tuck-svc,ou=service-accounts,dc=example,dc=com"
	BindDN string `json:"bind_dn"`

	// BindPassword is the service account password.
	// Never returned by GET /v1/auth/ldap/config.
	BindPassword string `json:"bind_password,omitempty"`

	// UserDN is the base DN for user searches.
	// Example: "ou=users,dc=example,dc=com"
	UserDN string `json:"user_dn"`

	// UserAttr is the attribute matched against the supplied username.
	// Default "uid". Use "sAMAccountName" for Active Directory.
	UserAttr string `json:"user_attr,omitempty"`

	// GroupDN, when set, enables an explicit group search in this base DN.
	// If empty, group membership is read from the groupAttr on the user entry.
	GroupDN string `json:"group_dn,omitempty"`

	// GroupAttr is the attribute read from the user entry to find group DNs.
	// Default "memberOf". Ignored when GroupDN is set.
	GroupAttr string `json:"group_attr,omitempty"`

	// GroupFilter is the LDAP filter template used when GroupDN is set.
	// The literal string "{{.UserDN}}" is replaced with the authenticated user's DN.
	// Default: "(member={{.UserDN}})"
	GroupFilter string `json:"group_filter,omitempty"`

	// TLSInsecure disables certificate verification. Never use in production.
	TLSInsecure bool `json:"tls_insecure,omitempty"`

	// StartTLS upgrades an ldap:// connection to TLS via the STARTTLS extension.
	StartTLS bool `json:"starttls,omitempty"`
}

func (c *Config) userAttrField() string {
	if c.UserAttr != "" {
		return c.UserAttr
	}
	return "uid"
}

func (c *Config) groupAttrField() string {
	if c.GroupAttr != "" {
		return c.GroupAttr
	}
	return "memberOf"
}

func (c *Config) buildGroupFilter(userDN string) string {
	f := c.GroupFilter
	if f == "" {
		f = "(member={{.UserDN}})"
	}
	return strings.ReplaceAll(f, "{{.UserDN}}", goldap.EscapeFilter(userDN))
}

// Role maps LDAP group membership or specific usernames to Tuck policies.
type Role struct {
	Name string `json:"name"`
	// Groups is a list of group CNs or full DNs that grant this role.
	// Matching is case-insensitive and accepts both the full DN and the CN
	// component (e.g. "admin" matches "CN=admin,OU=groups,DC=example,DC=com").
	Groups []string `json:"groups,omitempty"`
	// Users is a list of usernames or user DNs that always get this role,
	// regardless of group membership.
	Users []string `json:"users,omitempty"`
	// Policies are the Tuck policy names granted on login.
	Policies []string `json:"policies"`
	// TTL is the Tuck token lifetime. Zero = server default.
	TTL time.Duration `json:"ttl,omitempty"`
}

// LoginResult is returned by Authenticator.Login on success.
type LoginResult struct {
	UserDN   string
	Username string
	Policies []string
	TTL      time.Duration
}

// Entry is a single row from an LDAP search result.
type Entry struct {
	DN         string
	Attributes map[string][]string
}

// Conn is the subset of LDAP connection operations used by Authenticator.
// *goldap.Conn satisfies this interface; implement it to inject test stubs.
type Conn interface {
	Bind(dn, password string) error
	Search(baseDN, filter string, attrs []string) ([]*Entry, error)
	Close() error
}

// realConn wraps *goldap.Conn behind the Conn interface.
type realConn struct{ inner *goldap.Conn }

func (r *realConn) Bind(dn, password string) error { return r.inner.Bind(dn, password) }
func (r *realConn) Close() error                   { r.inner.Close(); return nil }

func (r *realConn) Search(baseDN, filter string, attrs []string) ([]*Entry, error) {
	req := goldap.NewSearchRequest(
		baseDN,
		goldap.ScopeWholeSubtree,
		goldap.NeverDerefAliases,
		0, 30, false,
		filter, attrs, nil,
	)
	res, err := r.inner.Search(req)
	if err != nil {
		return nil, err
	}
	out := make([]*Entry, 0, len(res.Entries))
	for _, e := range res.Entries {
		entry := &Entry{DN: e.DN, Attributes: make(map[string][]string, len(e.Attributes))}
		for _, a := range e.Attributes {
			entry.Attributes[a.Name] = a.Values
		}
		out = append(out, entry)
	}
	return out, nil
}

// --- barrier interface ---

type barrierIface interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// --- Store ---

// Store persists the LDAP Config and Roles inside the encrypted barrier.
type Store struct{ b barrierIface }

// NewStore creates a Store backed by b.
func NewStore(b barrierIface) *Store { return &Store{b: b} }

// GetConfig reads the LDAP config from the barrier.
func (s *Store) GetConfig(ctx context.Context) (*Config, error) {
	e, err := s.b.Get(ctx, configKey)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrNotConfigured
	}
	var cfg Config
	if err := json.Unmarshal(e.Value, &cfg); err != nil {
		return nil, fmt.Errorf("ldap: decode config: %w", err)
	}
	return &cfg, nil
}

// PutConfig writes the LDAP config into the barrier.
func (s *Store) PutConfig(ctx context.Context, cfg *Config) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("ldap: encode config: %w", err)
	}
	return s.b.Put(ctx, &physical.Entry{Key: configKey, Value: data})
}

// GetRole reads a role by name.
func (s *Store) GetRole(ctx context.Context, name string) (*Role, error) {
	e, err := s.b.Get(ctx, rolesKey+name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("ldap: role %q not found", name)
	}
	var r Role
	if err := json.Unmarshal(e.Value, &r); err != nil {
		return nil, fmt.Errorf("ldap: decode role: %w", err)
	}
	return &r, nil
}

// PutRole writes a role.
func (s *Store) PutRole(ctx context.Context, r *Role) error {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("ldap: encode role: %w", err)
	}
	return s.b.Put(ctx, &physical.Entry{Key: rolesKey + r.Name, Value: data})
}

// DeleteRole removes a role.
func (s *Store) DeleteRole(ctx context.Context, name string) error {
	return s.b.Delete(ctx, rolesKey+name)
}

// ListRoles returns all role names.
func (s *Store) ListRoles(ctx context.Context) ([]string, error) {
	keys, err := s.b.List(ctx, rolesKey)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, strings.TrimPrefix(k, rolesKey))
	}
	return out, nil
}

// AllRoles loads and returns every role.
func (s *Store) AllRoles(ctx context.Context) ([]*Role, error) {
	names, err := s.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	roles := make([]*Role, 0, len(names))
	for _, n := range names {
		r, err := s.GetRole(ctx, n)
		if err != nil {
			continue
		}
		roles = append(roles, r)
	}
	return roles, nil
}

// --- Authenticator ---

// Authenticator performs the LDAP operations that validate a login.
type Authenticator struct {
	cfg  Config
	dial func(ctx context.Context, cfg Config) (Conn, error)
}

// NewAuthenticator creates an Authenticator for the given config.
func NewAuthenticator(cfg Config, opts ...func(*Authenticator)) *Authenticator {
	a := &Authenticator{cfg: cfg, dial: defaultDial}
	for _, o := range opts {
		o(a)
	}
	return a
}

// WithDialer overrides the LDAP dial function. Use in tests to inject a fake Conn.
func WithDialer(dial func(ctx context.Context, cfg Config) (Conn, error)) func(*Authenticator) {
	return func(a *Authenticator) { a.dial = dial }
}

// Login validates username+password, resolves group membership, and matches
// the user against roles. Returns the union of policies from all matching roles.
func (a *Authenticator) Login(ctx context.Context, roles []*Role, username, password string) (*LoginResult, error) {
	conn, err := a.dial(ctx, a.cfg)
	if err != nil {
		return nil, fmt.Errorf("ldap: connect: %w", err)
	}
	defer conn.Close()

	if err := conn.Bind(a.cfg.BindDN, a.cfg.BindPassword); err != nil {
		return nil, fmt.Errorf("ldap: service account bind: %w", err)
	}

	userAttr := a.cfg.userAttrField()
	groupAttr := a.cfg.groupAttrField()
	filter := fmt.Sprintf("(%s=%s)", userAttr, goldap.EscapeFilter(username))
	entries, err := conn.Search(a.cfg.UserDN, filter, []string{"dn", groupAttr})
	if err != nil {
		return nil, fmt.Errorf("ldap: user search: %w", err)
	}
	if len(entries) == 0 {
		return nil, ErrInvalidCredentials
	}
	userDN := entries[0].DN
	userGroups := entries[0].Attributes[groupAttr]

	if err := conn.Bind(userDN, password); err != nil {
		return nil, ErrInvalidCredentials
	}

	if a.cfg.GroupDN != "" {
		if err := conn.Bind(a.cfg.BindDN, a.cfg.BindPassword); err != nil {
			return nil, fmt.Errorf("ldap: re-bind for group search: %w", err)
		}
		gf := a.cfg.buildGroupFilter(userDN)
		gEntries, err := conn.Search(a.cfg.GroupDN, gf, []string{"dn"})
		if err != nil {
			return nil, fmt.Errorf("ldap: group search: %w", err)
		}
		userGroups = make([]string, len(gEntries))
		for i, e := range gEntries {
			userGroups[i] = e.DN
		}
	}

	policies, ttl := matchRoles(roles, username, userDN, userGroups)
	if len(policies) == 0 {
		return nil, ErrNoRole
	}

	return &LoginResult{UserDN: userDN, Username: username, Policies: policies, TTL: ttl}, nil
}

// matchRoles returns the union of policies and the maximum TTL from all
// matching roles.
func matchRoles(roles []*Role, username, userDN string, groups []string) (policies []string, ttl time.Duration) {
	seen := make(map[string]bool)
	for _, role := range roles {
		if !roleMatches(role, username, userDN, groups) {
			continue
		}
		for _, p := range role.Policies {
			if !seen[p] {
				seen[p] = true
				policies = append(policies, p)
			}
		}
		if role.TTL > ttl {
			ttl = role.TTL
		}
	}
	return
}

func roleMatches(role *Role, username, userDN string, groups []string) bool {
	for _, u := range role.Users {
		if strings.EqualFold(u, username) || strings.EqualFold(u, userDN) {
			return true
		}
	}
	for _, g := range role.Groups {
		for _, ug := range groups {
			if strings.EqualFold(g, ug) || strings.EqualFold(g, cnOf(ug)) {
				return true
			}
		}
	}
	return false
}

// cnOf extracts the CN value from an LDAP DN.
// "CN=admin,OU=groups,DC=example,DC=com" → "admin"
func cnOf(dn string) string {
	first, _, _ := strings.Cut(dn, ",")
	k, v, ok := strings.Cut(first, "=")
	if ok && strings.EqualFold(k, "cn") {
		return v
	}
	return dn
}

// defaultDial connects to the first reachable LDAP server in cfg.URLs.
func defaultDial(_ context.Context, cfg Config) (Conn, error) {
	tlsCfg := &tls.Config{InsecureSkipVerify: cfg.TLSInsecure} // #nosec G402 — controlled by operator config field TLSInsecure
	var lastErr error
	for _, u := range cfg.URLs {
		conn, err := goldap.DialURL(u, goldap.DialWithTLSConfig(tlsCfg))
		if err != nil {
			lastErr = err
			continue
		}
		if cfg.StartTLS {
			if err := conn.StartTLS(tlsCfg); err != nil {
				conn.Close()
				lastErr = err
				continue
			}
		}
		return &realConn{inner: conn}, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("ldap: no server URLs configured")
}
