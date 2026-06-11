package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/NAGenaev/tuck/internal/barrier"
	"github.com/NAGenaev/tuck/internal/physical"
)

// ErrRoleNotFound is returned when a k8s role does not exist in the store.
var ErrRoleNotFound = errors.New("k8s role not found")

// RoleStore is a thin CRUD wrapper over a barrier for K8sRole persistence.
type RoleStore struct {
	barrier *barrier.Barrier
}

func NewRoleStore(b *barrier.Barrier) *RoleStore { return &RoleStore{barrier: b} }

func (s *RoleStore) Put(ctx context.Context, role *K8sRole) error {
	data, err := json.Marshal(role)
	if err != nil {
		return fmt.Errorf("marshal k8s role: %w", err)
	}
	return s.barrier.Put(ctx, &physical.Entry{Key: roleKey(role.Namespace, role.ServiceAccount), Value: data})
}

func (s *RoleStore) Get(ctx context.Context, namespace, sa string) (*K8sRole, error) {
	e, err := s.barrier.Get(ctx, roleKey(namespace, sa))
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrRoleNotFound
	}
	var role K8sRole
	if err := json.Unmarshal(e.Value, &role); err != nil {
		return nil, fmt.Errorf("unmarshal k8s role: %w", err)
	}
	return &role, nil
}

func (s *RoleStore) Delete(ctx context.Context, namespace, sa string) error {
	return s.barrier.Delete(ctx, roleKey(namespace, sa))
}

func roleKey(namespace, sa string) string {
	return "auth/k8s/role/" + namespace + "/" + sa
}
