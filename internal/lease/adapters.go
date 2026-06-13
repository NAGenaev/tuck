package lease

import (
	"context"
	"errors"
	"time"

	"github.com/NAGenaev/tuck/internal/dynamic/aws"
	"github.com/NAGenaev/tuck/internal/dynamic/azure"
	"github.com/NAGenaev/tuck/internal/dynamic/database"
	"github.com/NAGenaev/tuck/internal/dynamic/gcp"
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

// awsAdapter wraps *aws.Engine to implement Backend.
type awsAdapter struct{ e *aws.Engine }

func (a *awsAdapter) GetLeaseInfo(ctx context.Context, id string) (time.Time, bool, error) {
	l, err := a.e.GetLease(ctx, id)
	if err != nil {
		return time.Time{}, false, ErrNotFound
	}
	return l.ExpiresAt, l.Revoked, nil
}
func (a *awsAdapter) RevokeLease(ctx context.Context, id string) error {
	return a.e.RevokeLease(ctx, id)
}
func (a *awsAdapter) ListLeases(ctx context.Context) ([]string, error) { return a.e.ListLeases(ctx) }

// gcpAdapter wraps *gcp.Engine to implement Backend.
type gcpAdapter struct{ e *gcp.Engine }

func (a *gcpAdapter) GetLeaseInfo(ctx context.Context, id string) (time.Time, bool, error) {
	l, err := a.e.GetLease(ctx, id)
	if err != nil {
		return time.Time{}, false, ErrNotFound
	}
	return l.ExpiresAt, l.Revoked, nil
}
func (a *gcpAdapter) RevokeLease(ctx context.Context, id string) error {
	return a.e.RevokeLease(ctx, id)
}
func (a *gcpAdapter) ListLeases(ctx context.Context) ([]string, error) { return a.e.ListLeases(ctx) }

// azureAdapter wraps *azure.Engine to implement Backend.
type azureAdapter struct{ e *azure.Engine }

func (a *azureAdapter) GetLeaseInfo(ctx context.Context, id string) (time.Time, bool, error) {
	l, err := a.e.GetLease(ctx, id)
	if err != nil {
		return time.Time{}, false, ErrNotFound
	}
	return l.ExpiresAt, l.Revoked, nil
}
func (a *azureAdapter) RevokeLease(ctx context.Context, id string) error {
	return a.e.RevokeLease(ctx, id)
}
func (a *azureAdapter) ListLeases(ctx context.Context) ([]string, error) {
	return a.e.ListLeases(ctx)
}

// NewWithEngines creates a Manager wired to all four dynamic backend engines.
func NewWithEngines(db *database.Manager, awsE *aws.Engine, gcpE *gcp.Engine, azureE *azure.Engine) *Manager {
	return New(map[string]Backend{
		"database": &dbAdapter{m: db},
		"aws":      &awsAdapter{e: awsE},
		"gcp":      &gcpAdapter{e: gcpE},
		"azure":    &azureAdapter{e: azureE},
	})
}
