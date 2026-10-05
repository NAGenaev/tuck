package barrier

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

func openSuite(t *testing.T, be physical.Backend, name string) (*Barrier, []byte) {
	t.Helper()
	ctx := context.Background()
	root := randKey(t)
	b := New(be)
	if err := b.InitializeWithSuite(ctx, root, name); err != nil {
		t.Fatal(err)
	}
	if err := b.Unseal(ctx, root); err != nil {
		t.Fatal(err)
	}
	return b, root
}

func storedKeys(t *testing.T, be physical.Backend) []string {
	t.Helper()
	keys, err := be.List(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func TestPathsAreNotStoredInTheClear(t *testing.T) {
	forEachSuite(t, func(t *testing.T, name string) {
		ctx := context.Background()
		be := physical.NewInMem()
		b, _ := openSuite(t, be, name)
		for _, k := range []string{"secret/prod/db-password", "secret/prod/api-key", "auth/k8s/role/bank/payments"} {
			if err := b.Put(ctx, &physical.Entry{Key: k, Value: []byte("v")}); err != nil {
				t.Fatal(err)
			}
		}
		for _, k := range storedKeys(t, be) {
			if strings.HasPrefix(k, reservedPrefix) {
				continue
			}
			for _, leak := range []string{"secret", "prod", "db-password", "api-key", "auth", "k8s", "bank", "payments"} {
				if strings.Contains(k, leak) {
					t.Fatalf("stored key %q leaks %q", k, leak)
				}
			}
		}
	})
}

func TestPathRoundTripListAndPrefix(t *testing.T) {
	forEachSuite(t, func(t *testing.T, name string) {
		ctx := context.Background()
		be := physical.NewInMem()
		b, _ := openSuite(t, be, name)
		keys := []string{"secret/a/one", "secret/a/two", "secret/ab/three", "secret/b", "other/x"}
		for _, k := range keys {
			if err := b.Put(ctx, &physical.Entry{Key: k, Value: []byte(k)}); err != nil {
				t.Fatal(err)
			}
		}
		for _, k := range keys {
			e, err := b.Get(ctx, k)
			if err != nil || e == nil || string(e.Value) != k {
				t.Fatalf("get %s: %v %v", k, e, err)
			}
		}
		cases := map[string][]string{
			"secret/a/":  {"secret/a/one", "secret/a/two"},
			"secret/a":   {"secret/a/one", "secret/a/two", "secret/ab/three"},
			"secret/":    {"secret/a/one", "secret/a/two", "secret/ab/three", "secret/b"},
			"sec":        {"secret/a/one", "secret/a/two", "secret/ab/three", "secret/b"},
			"secret/zzz": nil,
			"other/":     {"other/x"},
		}
		for prefix, want := range cases {
			got, err := b.List(ctx, prefix)
			if err != nil {
				t.Fatalf("list %q: %v", prefix, err)
			}
			if len(got) == 0 && len(want) == 0 {
				continue
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("list %q = %v, want %v", prefix, got, want)
			}
		}
		if err := b.Delete(ctx, "secret/b"); err != nil {
			t.Fatal(err)
		}
		if e, _ := b.Get(ctx, "secret/b"); e != nil {
			t.Fatal("deleted entry still readable")
		}
	})
}

func TestPathEncodingDeterministicAndParentBound(t *testing.T) {
	forEachSuite(t, func(t *testing.T, name string) {
		b, _ := openSuite(t, physical.NewInMem(), name)
		a, c := b.phys("x/same"), b.phys("y/same")
		if a != b.phys("x/same") {
			t.Fatal("encoding must be deterministic")
		}
		if strings.SplitN(a, "/", 2)[1] == strings.SplitN(c, "/", 2)[1] {
			t.Fatal("the same segment under different parents must encode differently")
		}
	})
}

func TestMovedSegmentIsRejected(t *testing.T) {
	forEachSuite(t, func(t *testing.T, name string) {
		ctx := context.Background()
		be := physical.NewInMem()
		b, _ := openSuite(t, be, name)
		_ = b.Put(ctx, &physical.Entry{Key: "secret/low/k", Value: []byte("1")})
		_ = b.Put(ctx, &physical.Entry{Key: "secret/high/k", Value: []byte("2")})

		// Re-parent a stored segment: k from low moved under high.
		lowParts := strings.Split(b.phys("secret/low/k"), "/")
		highParts := strings.Split(b.phys("secret/high/k"), "/")
		forged := strings.Join([]string{highParts[0], highParts[1], lowParts[2]}, "/")
		if _, err := b.paths.decode(forged); err == nil {
			t.Fatal("a segment moved to another parent must not decode")
		}
	})
}

func TestPathEncryptionSurvivesSealRekeyAndRotation(t *testing.T) {
	forEachSuite(t, func(t *testing.T, name string) {
		ctx := context.Background()
		be := physical.NewInMem()
		b, root := openSuite(t, be, name)
		_ = b.Put(ctx, &physical.Entry{Key: "secret/a/one", Value: []byte("1")})
		stored := b.phys("secret/a/one")

		b.Seal()
		if b.paths != nil || b.pathKey != nil {
			t.Fatal("path key must be dropped on seal")
		}
		if err := b.Unseal(ctx, root); err != nil {
			t.Fatal(err)
		}
		if b.phys("secret/a/one") != stored {
			t.Fatal("stored key changed across unseal")
		}

		nr := randKey(t)
		if err := b.Rekey(ctx, nr); err != nil {
			t.Fatal(err)
		}
		b2 := New(be)
		if err := b2.Unseal(ctx, nr); err != nil {
			t.Fatal(err)
		}
		if b2.phys("secret/a/one") != stored {
			t.Fatal("rekey must keep the path key")
		}

		if _, n, err := b2.RotateDataKey(ctx, true); err != nil || n != 1 {
			t.Fatalf("rotate+reencrypt: %d %v", n, err)
		}
		if b2.phys("secret/a/one") != stored {
			t.Fatal("DEK rotation must not change stored keys")
		}
		e, err := b2.Get(ctx, "secret/a/one")
		if err != nil || e == nil || string(e.Value) != "1" {
			t.Fatalf("get after rotation: %v %v", e, err)
		}
		keys, err := b2.List(ctx, "secret/")
		if err != nil || !reflect.DeepEqual(keys, []string{"secret/a/one"}) {
			t.Fatalf("list after rotation: %v %v", keys, err)
		}
	})
}

// A store created before path encryption keeps plaintext keys.
func TestVersionedStoreWithoutPathEncryptionKeepsPlainKeys(t *testing.T) {
	ctx := context.Background()
	be := physical.NewInMem()
	root := randKey(t)
	b := New(be)
	if err := b.initialize(ctx, root, "", metaVersionVersionedKeys); err != nil {
		t.Fatal(err)
	}
	if err := b.Unseal(ctx, root); err != nil {
		t.Fatal(err)
	}
	if b.paths != nil {
		t.Fatal("v2 store must not use path encryption")
	}
	_ = b.Put(ctx, &physical.Entry{Key: "secret/a", Value: []byte("1")})
	if e, _ := be.Get(ctx, "secret/a"); e == nil {
		t.Fatal("v2 store must keep logical keys in the backend")
	}
	if keys, err := b.List(ctx, "secret/"); err != nil || !reflect.DeepEqual(keys, []string{"secret/a"}) {
		t.Fatalf("list: %v %v", keys, err)
	}
}
