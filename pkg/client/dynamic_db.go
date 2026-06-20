package client

import (
	"context"
	"time"
)

// DBClient manages the Database dynamic secrets engine.
type DBClient struct{ c *Client }

// DB returns a client for the Database dynamic secrets engine.
func (c *Client) DB() *DBClient { return &DBClient{c: c} }

// DBConfig holds connection parameters for a named database connection.
type DBConfig struct {
	Name          string `json:"name"`
	PluginName    string `json:"plugin_name"`    // "postgresql" or "mysql"
	ConnectionURL string `json:"connection_url"` // DSN
	MaxOpenConns  int    `json:"max_open_conns,omitempty"`
}

// DBRole defines how credentials are generated for a database connection.
type DBRole struct {
	Name                 string        `json:"name"`
	DBName               string        `json:"db_name"`
	CreationStatements   string        `json:"creation_statements"`
	RevocationStatements string        `json:"revocation_statements,omitempty"`
	DefaultTTL           time.Duration `json:"default_ttl,omitempty"`
	MaxTTL               time.Duration `json:"max_ttl,omitempty"`
}

// DBCredentials is returned when generating dynamic database credentials.
type DBCredentials struct {
	LeaseID   string        `json:"lease_id"`
	Username  string        `json:"username"`
	Password  string        `json:"password"`
	ExpiresAt time.Time     `json:"expires_at"`
	TTL       time.Duration `json:"ttl"`
}

// PutConfig creates or updates a named database connection config.
func (d *DBClient) PutConfig(ctx context.Context, cfg DBConfig) error {
	return d.c.putJSON(ctx, "/v1/database/config/"+cfg.Name, cfg)
}

// GetConfig reads a named database connection config.
func (d *DBClient) GetConfig(ctx context.Context, name string) (*DBConfig, error) {
	var out DBConfig
	if err := d.c.get(ctx, "/v1/database/config/"+name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteConfig removes a named database connection config.
func (d *DBClient) DeleteConfig(ctx context.Context, name string) error {
	return d.c.doDelete(ctx, "/v1/database/config/"+name)
}

// ListConfigs returns all named database connection config names.
func (d *DBClient) ListConfigs(ctx context.Context) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	if err := d.c.doList(ctx, "/v1/database/config/", &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

// PutRole creates or updates a database role.
func (d *DBClient) PutRole(ctx context.Context, role DBRole) error {
	return d.c.putJSON(ctx, "/v1/database/role/"+role.Name, role)
}

// GetRole reads a database role.
func (d *DBClient) GetRole(ctx context.Context, name string) (*DBRole, error) {
	var out DBRole
	if err := d.c.get(ctx, "/v1/database/role/"+name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRole removes a database role.
func (d *DBClient) DeleteRole(ctx context.Context, name string) error {
	return d.c.doDelete(ctx, "/v1/database/role/"+name)
}

// ListRoles returns all database role names.
func (d *DBClient) ListRoles(ctx context.Context) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	if err := d.c.doList(ctx, "/v1/database/role/", &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

// GenerateCreds generates a new set of dynamic database credentials for the named role.
func (d *DBClient) GenerateCreds(ctx context.Context, role string) (*DBCredentials, error) {
	var out DBCredentials
	if err := d.c.post(ctx, "/v1/database/creds/"+role, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeLease revokes a database credential lease before its TTL expires.
func (d *DBClient) RevokeLease(ctx context.Context, leaseID string) error {
	return d.c.doDelete(ctx, "/v1/database/lease/"+leaseID)
}
