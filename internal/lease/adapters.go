package lease

import (
	"context"
	"errors"
	"time"

	"github.com/NAGenaev/tuck/internal/dynamic/database"
)

// dbAdapter wraps *database.Manager to implement Backend.
type dbAdapter struct{ m *database.Manager }

func (a *dbAdapter) GetLeaseInfo(ctx context.Context, id string) (time.Time, bool, error) {
	l, err := a.m.GetLease(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return time.Time{}, false, ErrNotFound
		}
		return time.Time{}, false, err
	}
	return l.ExpiresAt, false, nil // database leases have no Revoked flag
}
func (a *dbAdapter) RevokeLease(ctx context.Context, id string) error {
	err := a.m.RevokeLease(ctx, id)
	if errors.Is(err, database.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
func (a *dbAdapter) ListLeases(ctx context.Context) ([]string, error) { return a.m.ListLeases(ctx) }
func (a *dbAdapter) RenewLease(ctx context.Context, id string, increment time.Duration) (time.Time, error) {
	return a.m.RenewLease(ctx, id, increment)
}

// NewWithEngines creates a Manager wired to the dynamic backend engines.
func NewWithEngines(db *database.Manager) *Manager {
	return New(map[string]Backend{
		"database": &dbAdapter{m: db},
	})
}
