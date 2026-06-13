// Package lease provides a unified view of all dynamic-credential leases
// across Tuck's dynamic backends (database, AWS, GCP, Azure).
// Each lease is addressable as "<backend>/<internal-id>", e.g. "database/abc123".
package lease

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned when no lease exists for the given ID.
var ErrNotFound = errors.New("lease: not found")

// Info is a backend-agnostic summary of a single lease.
type Info struct {
	ID        string    `json:"id"`         // unified: "<backend>/<internal-id>"
	Backend   string    `json:"backend"`    // e.g. "database", "aws"
	InternalID string   `json:"internal_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

// Backend is the subset of each dynamic engine used by the Manager.
type Backend interface {
	// GetLeaseInfo returns expiry and revoked status for the given internal ID.
	GetLeaseInfo(ctx context.Context, id string) (expiresAt time.Time, revoked bool, err error)
	// RevokeLease immediately revokes and deletes the lease.
	RevokeLease(ctx context.Context, id string) error
	// ListLeases returns all internal lease IDs known to this backend.
	ListLeases(ctx context.Context) ([]string, error)
}

// Manager federates lease operations across multiple named backends.
type Manager struct {
	backends map[string]Backend
}

// New creates a Manager with the provided backends.
func New(backends map[string]Backend) *Manager {
	return &Manager{backends: backends}
}

// Lookup returns unified lease Info for a unified lease ID (format: "backend/id").
func (m *Manager) Lookup(ctx context.Context, unifiedID string) (*Info, error) {
	name, internalID, err := splitID(unifiedID)
	if err != nil {
		return nil, ErrNotFound
	}
	b, ok := m.backends[name]
	if !ok {
		return nil, ErrNotFound
	}
	expiresAt, revoked, err := b.GetLeaseInfo(ctx, internalID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lease lookup %q: %w", unifiedID, err)
	}
	return &Info{
		ID:         unifiedID,
		Backend:    name,
		InternalID: internalID,
		ExpiresAt:  expiresAt,
		Revoked:    revoked,
	}, nil
}

// Revoke revokes the lease identified by unifiedID.
func (m *Manager) Revoke(ctx context.Context, unifiedID string) error {
	name, internalID, err := splitID(unifiedID)
	if err != nil {
		return ErrNotFound
	}
	b, ok := m.backends[name]
	if !ok {
		return ErrNotFound
	}
	return b.RevokeLease(ctx, internalID)
}

// List returns Info for all leases across all backends.
func (m *Manager) List(ctx context.Context) ([]*Info, error) {
	var out []*Info
	for name, b := range m.backends {
		ids, err := b.ListLeases(ctx)
		if err != nil {
			return nil, fmt.Errorf("list %s leases: %w", name, err)
		}
		for _, id := range ids {
			expiresAt, revoked, gErr := b.GetLeaseInfo(ctx, id)
			if gErr != nil {
				continue // skip leases that can't be read
			}
			out = append(out, &Info{
				ID:         name + "/" + id,
				Backend:    name,
				InternalID: id,
				ExpiresAt:  expiresAt,
				Revoked:    revoked,
			})
		}
	}
	return out, nil
}

func splitID(id string) (backend, internalID string, err error) {
	idx := strings.Index(id, "/")
	if idx <= 0 || idx == len(id)-1 {
		return "", "", fmt.Errorf("invalid lease id %q", id)
	}
	return id[:idx], id[idx+1:], nil
}
