// Package database implements the Tuck dynamic secrets engine for relational
// databases. On each credential request Tuck connects to the configured
// database, creates a short-lived user with the SQL from the role's
// creation_statements, and records a lease. When the lease expires the user
// is dropped via revocation_statements.
//
// Supported database types: "postgresql", "mysql".
package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql" // register mysql driver
	_ "github.com/lib/pq"              // register postgres driver

	"github.com/NAGenaev/tuck/internal/physical"
)

var (
	// ErrNotFound is returned when a config, role, or lease does not exist.
	ErrNotFound = errors.New("database: not found")
	// ErrLeaseExpired is returned when a lease has already expired.
	ErrLeaseExpired = errors.New("database: lease expired")
)

const (
	configsKey = "dynamic/database/configs/"
	rolesKey   = "dynamic/database/roles/"
	leasesKey  = "dynamic/database/leases/"

	defaultDefaultTTL = 1 * time.Hour
	defaultMaxTTL     = 24 * time.Hour

	// DefaultCreationStatementsPostgres is a safe template for PostgreSQL.
	DefaultCreationStatementsPostgres = `CREATE USER "{{username}}" WITH PASSWORD '{{password}}' VALID UNTIL '{{expiry}}'; GRANT CONNECT ON DATABASE {{database}} TO "{{username}}";`
	// DefaultRevocationStatementsPostgres revokes and drops the user.
	DefaultRevocationStatementsPostgres = `REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM "{{username}}"; DROP USER IF EXISTS "{{username}}";`
	// DefaultCreationStatementsMySQL is a safe template for MySQL.
	DefaultCreationStatementsMySQL = `CREATE USER '{{username}}'@'%' IDENTIFIED BY '{{password}}'; GRANT SELECT ON {{database}}.* TO '{{username}}'@'%';`
	// DefaultRevocationStatementsMySQL drops the MySQL user.
	DefaultRevocationStatementsMySQL = `DROP USER IF EXISTS '{{username}}'@'%';`
)

// Config holds a named database connection.
type Config struct {
	Name          string `json:"name"`
	// PluginName is "postgresql" or "mysql".
	PluginName    string `json:"plugin_name"`
	// ConnectionURL is the DSN (may contain {{username}} and {{password}} for rotation).
	ConnectionURL string `json:"connection_url"`
	// MaxOpenConns defaults to 5.
	MaxOpenConns  int    `json:"max_open_conns,omitempty"`
}

// Role defines how credentials are generated for a named database.
type Role struct {
	Name                string        `json:"name"`
	DBName              string        `json:"db_name"`
	// CreationStatements is SQL executed to create the dynamic user.
	// Template vars: {{username}}, {{password}}, {{expiry}}, {{database}}.
	CreationStatements  string        `json:"creation_statements"`
	// RevocationStatements is SQL executed on lease expiry/revocation.
	RevocationStatements string       `json:"revocation_statements"`
	DefaultTTL          time.Duration `json:"default_ttl,omitempty"`
	MaxTTL              time.Duration `json:"max_ttl,omitempty"`
}

// Lease tracks a single set of generated credentials.
type Lease struct {
	ID        string    `json:"id"`
	RoleName  string    `json:"role_name"`
	DBName    string    `json:"db_name"`
	Username  string    `json:"username"`
	// Password is stored encrypted (inside the barrier) so the engine can
	// re-use it during revocation if needed.
	Password  string    `json:"password"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsExpired reports whether the lease has passed its ExpiresAt time.
func (l *Lease) IsExpired() bool { return time.Now().After(l.ExpiresAt) }

// Credentials is returned on a successful credential request.
type Credentials struct {
	LeaseID   string        `json:"lease_id"`
	Username  string        `json:"username"`
	Password  string        `json:"password"`
	ExpiresAt time.Time     `json:"expires_at"`
	TTL       time.Duration `json:"ttl"`
}

type barrier interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Manager manages database configs, roles, leases, and open DB connections.
type Manager struct {
	b    barrier
	mu   sync.Mutex
	pool map[string]*sql.DB // name → open connection
}

// NewManager creates a Manager backed by the given barrier.
func NewManager(b barrier) *Manager {
	return &Manager{b: b, pool: make(map[string]*sql.DB)}
}

// --- Config CRUD ---

func (m *Manager) PutConfig(ctx context.Context, cfg *Config) error {
	return m.put(ctx, configsKey+cfg.Name, cfg)
}

func (m *Manager) GetConfig(ctx context.Context, name string) (*Config, error) {
	var cfg Config
	if err := m.get(ctx, configsKey+name, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (m *Manager) DeleteConfig(ctx context.Context, name string) error {
	m.mu.Lock()
	if db, ok := m.pool[name]; ok {
		db.Close()
		delete(m.pool, name)
	}
	m.mu.Unlock()
	return m.b.Delete(ctx, configsKey+name)
}

func (m *Manager) ListConfigs(ctx context.Context) ([]string, error) {
	return m.listTrimmed(ctx, configsKey)
}

// --- Role CRUD ---

func (m *Manager) PutRole(ctx context.Context, r *Role) error {
	if r.DefaultTTL <= 0 {
		r.DefaultTTL = defaultDefaultTTL
	}
	if r.MaxTTL <= 0 {
		r.MaxTTL = defaultMaxTTL
	}
	if r.CreationStatements == "" {
		cfg, err := m.GetConfig(ctx, r.DBName)
		if err == nil {
			switch cfg.PluginName {
			case "postgresql":
				r.CreationStatements = DefaultCreationStatementsPostgres
			case "mysql":
				r.CreationStatements = DefaultCreationStatementsMySQL
			}
		}
	}
	if r.RevocationStatements == "" {
		cfg, err := m.GetConfig(ctx, r.DBName)
		if err == nil {
			switch cfg.PluginName {
			case "postgresql":
				r.RevocationStatements = DefaultRevocationStatementsPostgres
			case "mysql":
				r.RevocationStatements = DefaultRevocationStatementsMySQL
			}
		}
	}
	return m.put(ctx, rolesKey+r.Name, r)
}

func (m *Manager) GetRole(ctx context.Context, name string) (*Role, error) {
	var r Role
	if err := m.get(ctx, rolesKey+name, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (m *Manager) DeleteRole(ctx context.Context, name string) error {
	return m.b.Delete(ctx, rolesKey+name)
}

func (m *Manager) ListRoles(ctx context.Context) ([]string, error) {
	return m.listTrimmed(ctx, rolesKey)
}

// --- Credential generation ---

// GenerateCreds creates a new database user for the named role and returns
// a Credentials value containing the username, password, and lease ID.
func (m *Manager) GenerateCreds(ctx context.Context, roleName string) (*Credentials, error) {
	role, err := m.GetRole(ctx, roleName)
	if err != nil {
		return nil, err
	}
	cfg, err := m.GetConfig(ctx, role.DBName)
	if err != nil {
		return nil, err
	}

	db, err := m.openDB(cfg)
	if err != nil {
		return nil, fmt.Errorf("database: connect %q: %w", cfg.Name, err)
	}

	username := fmt.Sprintf("tuck_%s_%d", sanitize(roleName), time.Now().UnixMilli())
	password, err := generatePassword(16)
	if err != nil {
		return nil, fmt.Errorf("database: generate password: %w", err)
	}

	ttl := role.DefaultTTL
	if ttl <= 0 {
		ttl = defaultDefaultTTL
	}
	expiresAt := time.Now().UTC().Add(ttl)

	stmts := renderTemplate(role.CreationStatements, map[string]string{
		"{{username}}": username,
		"{{password}}": password,
		"{{expiry}}":   expiresAt.Format(time.RFC3339),
		"{{database}}": cfg.Name,
	})

	if err := execStatements(ctx, db, stmts); err != nil {
		return nil, fmt.Errorf("database: create user: %w", err)
	}

	leaseID, err := generateID()
	if err != nil {
		return nil, err
	}
	lease := &Lease{
		ID:        leaseID,
		RoleName:  roleName,
		DBName:    cfg.Name,
		Username:  username,
		Password:  password,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
	}
	if err := m.put(ctx, leasesKey+leaseID, lease); err != nil {
		return nil, err
	}

	return &Credentials{
		LeaseID:   leaseID,
		Username:  username,
		Password:  password,
		ExpiresAt: expiresAt,
		TTL:       ttl,
	}, nil
}

// RevokeLease immediately revokes the credentials for leaseID.
func (m *Manager) RevokeLease(ctx context.Context, leaseID string) error {
	var lease Lease
	if err := m.get(ctx, leasesKey+leaseID, &lease); err != nil {
		return err
	}
	return m.revokeLease(ctx, &lease)
}

// RenewLease extends the lease TTL by increment.
// The new ExpiresAt is capped at CreatedAt + role.MaxTTL when MaxTTL > 0.
func (m *Manager) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (time.Time, error) {
	var l Lease
	if err := m.get(ctx, leasesKey+leaseID, &l); err != nil {
		return time.Time{}, err
	}
	if l.IsExpired() {
		return time.Time{}, ErrLeaseExpired
	}
	newExpiry := time.Now().Add(increment)
	if role, err := m.GetRole(ctx, l.RoleName); err == nil && role.MaxTTL > 0 {
		if cap := l.CreatedAt.Add(role.MaxTTL); newExpiry.After(cap) {
			newExpiry = cap
		}
	}
	l.ExpiresAt = newExpiry
	if err := m.put(ctx, leasesKey+leaseID, &l); err != nil {
		return time.Time{}, err
	}
	return l.ExpiresAt, nil
}

// RevokeExpired revokes all leases whose ExpiresAt is before now.
// Call this from the background GC loop.
func (m *Manager) RevokeExpired(ctx context.Context) error {
	keys, err := m.b.List(ctx, leasesKey)
	if err != nil {
		return err
	}
	for _, k := range keys {
		var lease Lease
		if err := m.get(ctx, k, &lease); err != nil {
			continue
		}
		if lease.IsExpired() {
			_ = m.revokeLease(ctx, &lease)
		}
	}
	return nil
}

func (m *Manager) revokeLease(ctx context.Context, lease *Lease) error {
	cfg, err := m.GetConfig(ctx, lease.DBName)
	if err != nil {
		// Config deleted — just remove the lease record.
		return m.b.Delete(ctx, leasesKey+lease.ID)
	}
	role, _ := m.GetRole(ctx, lease.RoleName)

	var stmts string
	if role != nil && role.RevocationStatements != "" {
		stmts = renderTemplate(role.RevocationStatements, map[string]string{
			"{{username}}": lease.Username,
			"{{database}}": cfg.Name,
		})
	}
	if stmts != "" {
		if db, err := m.openDB(cfg); err == nil {
			_ = execStatements(ctx, db, stmts)
		}
	}
	return m.b.Delete(ctx, leasesKey+lease.ID)
}

// GetLease returns the lease record for introspection.
func (m *Manager) GetLease(ctx context.Context, leaseID string) (*Lease, error) {
	var l Lease
	if err := m.get(ctx, leasesKey+leaseID, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// ListLeases returns all active lease IDs.
func (m *Manager) ListLeases(ctx context.Context) ([]string, error) {
	return m.listTrimmed(ctx, leasesKey)
}

// Close closes all open database connections.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, db := range m.pool {
		db.Close()
	}
	m.pool = make(map[string]*sql.DB)
}

// --- internal helpers ---

func (m *Manager) openDB(cfg *Config) (*sql.DB, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if db, ok := m.pool[cfg.Name]; ok {
		if err := db.Ping(); err == nil {
			return db, nil
		}
		db.Close()
		delete(m.pool, cfg.Name)
	}

	driver := driverName(cfg.PluginName)
	if driver == "" {
		return nil, fmt.Errorf("database: unsupported plugin %q", cfg.PluginName)
	}
	db, err := sql.Open(driver, cfg.ConnectionURL)
	if err != nil {
		return nil, err
	}
	max := cfg.MaxOpenConns
	if max <= 0 {
		max = 5
	}
	db.SetMaxOpenConns(max)
	db.SetConnMaxLifetime(5 * time.Minute)
	m.pool[cfg.Name] = db
	return db, nil
}

func driverName(plugin string) string {
	switch plugin {
	case "postgresql":
		return "postgres"
	case "mysql":
		return "mysql"
	}
	return ""
}

func execStatements(ctx context.Context, db *sql.DB, stmts string) error {
	for _, stmt := range splitStatements(stmts) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt, err)
		}
	}
	return nil
}

func splitStatements(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func renderTemplate(tmpl string, vars map[string]string) string {
	for k, v := range vars {
		tmpl = strings.ReplaceAll(tmpl, k, v)
	}
	return tmpl
}

// sanitize removes characters unsafe for SQL identifiers.
func sanitize(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func generatePassword(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func generateID() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (m *Manager) get(ctx context.Context, key string, dst interface{}) error {
	e, err := m.b.Get(ctx, key)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNotFound
	}
	return json.Unmarshal(e.Value, dst)
}

func (m *Manager) put(ctx context.Context, key string, src interface{}) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return m.b.Put(ctx, &physical.Entry{Key: key, Value: data})
}

func (m *Manager) listTrimmed(ctx context.Context, prefix string) ([]string, error) {
	keys, err := m.b.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	for i, k := range keys {
		keys[i] = strings.TrimPrefix(k, prefix)
	}
	return keys, nil
}
