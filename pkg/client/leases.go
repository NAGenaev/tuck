package client

import (
	"context"
	"time"
)

// LeaseClient manages system-level lease operations.
type LeaseClient struct{ c *Client }

// Leases returns a client for lease management.
func (c *Client) Leases() *LeaseClient { return &LeaseClient{c: c} }

// Lease describes a dynamic secret lease.
type Lease struct {
	ID        string        `json:"id"`
	TTL       time.Duration `json:"ttl"`
	Renewable bool          `json:"renewable"`
	ExpiresAt time.Time     `json:"expires_at,omitempty"`
}

// Get returns metadata for a lease by ID.
func (l *LeaseClient) Get(ctx context.Context, id string) (*Lease, error) {
	var out Lease
	if err := l.c.get(ctx, "/v1/sys/leases/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Renew extends a lease. ttl=0 uses the lease's default increment.
func (l *LeaseClient) Renew(ctx context.Context, id string, ttl time.Duration) (*Lease, error) {
	body := map[string]any{"lease_id": id}
	if ttl > 0 {
		body["increment"] = int(ttl.Seconds())
	}
	var out Lease
	if err := l.c.post(ctx, "/v1/sys/leases/renew", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Revoke immediately revokes a lease.
func (l *LeaseClient) Revoke(ctx context.Context, id string) error {
	return l.c.doDelete(ctx, "/v1/sys/leases/"+id)
}

// List returns all active lease IDs.
func (l *LeaseClient) List(ctx context.Context) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	if err := l.c.doList(ctx, "/v1/sys/leases/", &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}
