package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

func newTestCatalog(t *testing.T) *Catalog {
	t.Helper()
	return New(physical.NewInMem())
}

func sampleEntry(t PluginType, name string) *Entry {
	return &Entry{
		Name:    name,
		Type:    t,
		Command: "/opt/plugins/" + name,
		SHA256:  "deadbeefdeadbeefdeadbeef",
	}
}

func TestRegister_and_Get(t *testing.T) {
	c := newTestCatalog(t)
	ctx := context.Background()

	e := sampleEntry(TypeSecret, "my-plugin")
	if err := c.Register(ctx, e); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get(ctx, TypeSecret, "my-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "my-plugin" || got.Type != TypeSecret {
		t.Errorf("got %+v", got)
	}
	if got.Builtin {
		t.Error("registered plugin must not be builtin")
	}
	if got.RegisteredAt.IsZero() {
		t.Error("registered_at must be set")
	}
}

func TestRegister_invalidType(t *testing.T) {
	c := newTestCatalog(t)
	e := sampleEntry("unknown", "p")
	err := c.Register(context.Background(), e)
	if !errors.Is(err, ErrInvalidType) {
		t.Fatalf("err = %v, want ErrInvalidType", err)
	}
}

func TestRegister_invalidName(t *testing.T) {
	c := newTestCatalog(t)
	e := sampleEntry(TypeAuth, "bad/name")
	err := c.Register(context.Background(), e)
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("err = %v, want ErrInvalidName", err)
	}
}

func TestRegister_emptyName(t *testing.T) {
	c := newTestCatalog(t)
	e := sampleEntry(TypeAuth, "")
	err := c.Register(context.Background(), e)
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("err = %v, want ErrInvalidName", err)
	}
}

func TestGet_notFound(t *testing.T) {
	c := newTestCatalog(t)
	_, err := c.Get(context.Background(), TypeSecret, "ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDelete(t *testing.T) {
	c := newTestCatalog(t)
	ctx := context.Background()

	e := sampleEntry(TypeDatabase, "pg-plugin")
	if err := c.Register(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, TypeDatabase, "pg-plugin"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, TypeDatabase, "pg-plugin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDelete_notFound(t *testing.T) {
	c := newTestCatalog(t)
	err := c.Delete(context.Background(), TypeSecret, "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRegisterBuiltin_idempotent(t *testing.T) {
	c := newTestCatalog(t)
	ctx := context.Background()

	e := sampleEntry(TypeSecret, "builtin-engine")
	if err := c.RegisterBuiltin(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := c.RegisterBuiltin(ctx, e); err != nil {
		t.Fatalf("second RegisterBuiltin: %v", err)
	}
	got, _ := c.Get(ctx, TypeSecret, "builtin-engine")
	if !got.Builtin {
		t.Error("builtin plugin must have Builtin=true")
	}
}

func TestList_byType(t *testing.T) {
	c := newTestCatalog(t)
	ctx := context.Background()

	_ = c.Register(ctx, sampleEntry(TypeSecret, "s1"))
	_ = c.Register(ctx, sampleEntry(TypeSecret, "s2"))
	_ = c.Register(ctx, sampleEntry(TypeAuth, "a1"))

	secPlugins, err := c.List(ctx, TypeSecret)
	if err != nil {
		t.Fatal(err)
	}
	if len(secPlugins) != 2 {
		t.Errorf("secret plugins = %d, want 2", len(secPlugins))
	}

	authPlugins, err := c.List(ctx, TypeAuth)
	if err != nil {
		t.Fatal(err)
	}
	if len(authPlugins) != 1 {
		t.Errorf("auth plugins = %d, want 1", len(authPlugins))
	}
}

func TestList_allTypes(t *testing.T) {
	c := newTestCatalog(t)
	ctx := context.Background()

	_ = c.Register(ctx, sampleEntry(TypeSecret, "s1"))
	_ = c.Register(ctx, sampleEntry(TypeAuth, "a1"))
	_ = c.Register(ctx, sampleEntry(TypeDatabase, "d1"))

	all, err := c.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("all plugins = %d, want 3", len(all))
	}
}
