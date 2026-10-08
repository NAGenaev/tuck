package core

import (
	"context"
	"fmt"
	"time"

	"github.com/NAGenaev/tuck/internal/namespace"
)

// CreateNamespaceWithMetadata is CreateNamespace plus description, owner and
// labels. The metadata is validated before anything is stored.
func (c *Core) CreateNamespaceWithMetadata(ctx context.Context, name string, m namespace.Metadata) (*namespace.Namespace, error) {
	if err := namespace.ValidateName(name); err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if existing, err := c.namespaces.Get(ctx, name); err == nil && existing != nil {
		return nil, fmt.Errorf("%w: %q", namespace.ErrAlreadyExists, name)
	}
	ns := &namespace.Namespace{Name: name, CreatedAt: time.Now().UTC()}
	ns.Apply(m)
	if err := c.namespaces.Put(ctx, ns); err != nil {
		return nil, err
	}
	return ns, nil
}

// UpdateNamespaceMetadata replaces the description, owner and labels of an
// existing namespace. The name and creation time never change.
func (c *Core) UpdateNamespaceMetadata(ctx context.Context, name string, m namespace.Metadata) (*namespace.Namespace, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	ns, err := c.namespaces.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	ns.Apply(m)
	ns.UpdatedAt = time.Now().UTC()
	if err := c.namespaces.Put(ctx, ns); err != nil {
		return nil, err
	}
	return ns, nil
}
