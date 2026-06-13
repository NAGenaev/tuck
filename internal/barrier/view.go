package barrier

import (
	"context"

	"github.com/NAGenaev/tuck/internal/physical"
)

// View is a prefix-scoped read-write view of a Barrier.
// All keys passed to Get/Put/Delete/List are prepended with the view's
// prefix, so each namespace can share a single Barrier without key collisions.
type View struct {
	barrier *Barrier
	prefix  string
}

// View returns a new View that prepends prefix to every barrier key.
func (b *Barrier) View(prefix string) *View {
	return &View{barrier: b, prefix: prefix}
}

func (v *View) Get(ctx context.Context, key string) (*physical.Entry, error) {
	e, err := v.barrier.Get(ctx, v.prefix+key)
	if err != nil || e == nil {
		return nil, err
	}
	return &physical.Entry{Key: key, Value: e.Value}, nil
}

func (v *View) Put(ctx context.Context, entry *physical.Entry) error {
	return v.barrier.Put(ctx, &physical.Entry{Key: v.prefix + entry.Key, Value: entry.Value})
}

func (v *View) Delete(ctx context.Context, key string) error {
	return v.barrier.Delete(ctx, v.prefix+key)
}

func (v *View) List(ctx context.Context, prefix string) ([]string, error) {
	return v.barrier.List(ctx, v.prefix+prefix)
}
