package operator

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func multiTS(ns, name, secretName string, data map[string]string) TuckSecret {
	return TuckSecret{
		Metadata: ObjectMeta{Name: name, Namespace: ns, ResourceVersion: "1"},
		Spec:     TuckSecretSpec{SecretName: secretName, Data: data},
	}
}

func TestReconcile_MultiKeyData(t *testing.T) {
	kube := &mockKube{}
	tuck := &mockTuck{secrets: map[string][]byte{"app/user": []byte("u"), "app/pass": []byte("p"), "app/legacy": []byte("l")}}
	ctrl := New(kube, tuck, "")

	ts := multiTS("prod", "creds", "app-creds", map[string]string{"username": "app/user", "password": "app/pass"})
	ts.Spec.TuckPath, ts.Spec.SecretKey = "app/legacy", "legacy"
	if err := ctrl.reconcile(context.Background(), ts); err != nil {
		t.Fatal(err)
	}
	if len(kube.applied) != 1 {
		t.Fatalf("applied = %d, want 1", len(kube.applied))
	}
	d := kube.applied[0].Data
	if string(d["username"]) != "u" || string(d["password"]) != "p" || string(d["legacy"]) != "l" || len(d) != 3 {
		t.Errorf("unexpected data %v", d)
	}
}

func TestReconcile_InvalidSpecs(t *testing.T) {
	cases := map[string]TuckSecretSpec{
		"nothing declared":   {SecretName: "s"},
		"path without key":   {SecretName: "s", TuckPath: "a/b"},
		"key without path":   {SecretName: "s", SecretKey: "k"},
		"empty data path":    {SecretName: "s", Data: map[string]string{"k": ""}},
		"duplicate key":      {SecretName: "s", TuckPath: "a", SecretKey: "k", Data: map[string]string{"k": "b"}},
		"missing secretName": {Data: map[string]string{"k": "a"}},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			kube := &mockKube{}
			ctrl := New(kube, &mockTuck{}, "")
			ts := TuckSecret{Metadata: ObjectMeta{Name: "x", Namespace: "ns"}, Spec: spec}
			if err := ctrl.reconcile(context.Background(), ts); err == nil {
				t.Fatal("expected error")
			}
			if len(kube.applied) != 0 {
				t.Error("secret must not be applied for an invalid spec")
			}
		})
	}
}

type failingTuck struct{ failPath string }

func (f failingTuck) GetSecret(_ context.Context, p, _ string) ([]byte, error) {
	if p == f.failPath {
		return nil, errors.New("boom")
	}
	return []byte("v"), nil
}

func TestReconcile_PartialFetchFailureAppliesNothing(t *testing.T) {
	kube := &mockKube{}
	ctrl := New(kube, failingTuck{failPath: "b"}, "")
	ts := multiTS("ns", "x", "s", map[string]string{"a": "a", "b": "b"})
	if err := ctrl.reconcile(context.Background(), ts); err == nil {
		t.Fatal("expected error")
	}
	if len(kube.applied) != 0 {
		t.Error("Secret must not be written when any key fails to fetch")
	}
}

func TestReconcile_SameSecretDisjointKeysAllowed(t *testing.T) {
	kube := &mockKube{}
	tuck := &mockTuck{secrets: map[string][]byte{"a": []byte("1"), "b": []byte("2")}}
	ctrl := New(kube, tuck, "")
	one := multiTS("ns", "one", "shared", map[string]string{"k1": "a"})
	two := multiTS("ns", "two", "shared", map[string]string{"k2": "b"})
	ctrl.addTracked(one)
	ctrl.addTracked(two)
	if err := ctrl.reconcile(context.Background(), one); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.reconcile(context.Background(), two); err != nil {
		t.Fatal(err)
	}
}

func TestReconcile_SameSecretSameKeyConflicts(t *testing.T) {
	kube := &mockKube{}
	tuck := &mockTuck{secrets: map[string][]byte{"a": []byte("1"), "b": []byte("2")}}
	ctrl := New(kube, tuck, "")
	one := multiTS("ns", "one", "shared", map[string]string{"k": "a"})
	two := multiTS("ns", "two", "shared", map[string]string{"k": "b"})
	ctrl.addTracked(one)
	ctrl.addTracked(two)
	err := ctrl.reconcile(context.Background(), two)
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if len(kube.applied) != 0 {
		t.Error("conflicting TuckSecret must not write the Secret")
	}
	// Same Secret name in another namespace is a different Secret.
	other := multiTS("other", "two", "shared", map[string]string{"k": "b"})
	ctrl.addTracked(other)
	if err := ctrl.reconcile(context.Background(), other); err != nil {
		t.Fatalf("different namespace must not conflict: %v", err)
	}
}

func TestMergeSecret(t *testing.T) {
	desired := &KubeSecret{
		Metadata: ObjectMeta{Labels: map[string]string{"managed-by": "tuck-operator"}},
		Data:     map[string][]byte{"k1": []byte("new")},
	}
	existing := &KubeSecret{
		Type: "kubernetes.io/basic-auth",
		Metadata: ObjectMeta{
			ResourceVersion: "42",
			Labels:          map[string]string{"app": "x"},
			Annotations:     map[string]string{"a": "b"},
		},
		Data: map[string][]byte{"k1": []byte("old"), "k2": []byte("keep")},
	}
	mergeSecret(desired, existing)
	if string(desired.Data["k1"]) != "new" || string(desired.Data["k2"]) != "keep" {
		t.Errorf("data = %v", desired.Data)
	}
	if desired.Metadata.ResourceVersion != "42" || desired.Type != "kubernetes.io/basic-auth" {
		t.Errorf("resourceVersion/type not carried over: %+v", desired)
	}
	if desired.Metadata.Labels["app"] != "x" || desired.Metadata.Labels["managed-by"] != "tuck-operator" {
		t.Errorf("labels = %v", desired.Metadata.Labels)
	}
	if desired.Metadata.Annotations["a"] != "b" {
		t.Errorf("annotations = %v", desired.Metadata.Annotations)
	}
}

func enforced(kube *mockKube, tuck TuckClientIface) *Controller {
	c := New(kube, tuck, "")
	c.EnforceNamespacePaths(true)
	return c
}

func TestEnforce_AllowsOwnNamespacePrefix(t *testing.T) {
	kube := &mockKube{}
	tuck := &mockTuck{secrets: map[string][]byte{"team-a/db/pass": []byte("p")}}
	ts := multiTS("team-a", "x", "s", map[string]string{"pw": "/team-a/db/pass"})
	if err := enforced(kube, tuck).reconcile(context.Background(), ts); err != nil {
		t.Fatal(err)
	}
	if len(tuck.fetched) != 1 || tuck.fetched[0] != "team-a/db/pass" {
		t.Errorf("fetched = %v", tuck.fetched)
	}
}

func TestEnforce_DeniesForeignAndTraversalPaths(t *testing.T) {
	bad := []string{
		"team-b/db/pass",
		"team-ab/x",
		"team-a/../team-b/x",
		"team-a/./x",
		"team-a//x",
		"team-a/x?y=1",
		"team-a/x#frag",
		"team-a/%2e%2e/team-b",
		"team-a\\..\\team-b",
		"secret-root",
		"team-a ",
		"",
	}
	for _, p := range bad {
		t.Run(p, func(t *testing.T) {
			kube := &mockKube{}
			tuck := &mockTuck{secrets: map[string][]byte{}}
			ts := multiTS("team-a", "x", "s", map[string]string{"k": p})
			if err := enforced(kube, tuck).reconcile(context.Background(), ts); err == nil {
				t.Fatalf("path %q must be rejected", p)
			}
			if len(tuck.fetched) != 0 || len(kube.applied) != 0 {
				t.Errorf("nothing may be fetched or written for %q", p)
			}
		})
	}
}

func TestEnforce_NamespaceAnnotationExtendsAccess(t *testing.T) {
	kube := &mockKube{nsAnnotations: map[string]map[string]string{
		"team-a": {AnnotationAllowedPaths: "shared/tls/, platform/common"},
	}}
	tuck := &mockTuck{secrets: map[string][]byte{"shared/tls/cert": []byte("c"), "platform/common/x": []byte("x"), "shared/other": []byte("o")}}
	ctrl := enforced(kube, tuck)

	ok := multiTS("team-a", "ok", "s", map[string]string{"cert": "shared/tls/cert", "x": "platform/common/x"})
	if err := ctrl.reconcile(context.Background(), ok); err != nil {
		t.Fatal(err)
	}
	denied := multiTS("team-a", "no", "s2", map[string]string{"o": "shared/other"})
	if err := ctrl.reconcile(context.Background(), denied); err == nil {
		t.Fatal("shared/other is outside the granted prefixes")
	}
	other := multiTS("team-b", "b", "s3", map[string]string{"cert": "shared/tls/cert"})
	if err := ctrl.reconcile(context.Background(), other); err == nil {
		t.Fatal("team-b must not inherit team-a's annotation")
	}
}

func TestEnforce_FailsClosedWhenNamespaceUnreadable(t *testing.T) {
	kube := &mockKube{nsErr: errors.New("forbidden")}
	tuck := &mockTuck{secrets: map[string][]byte{"team-a/x": []byte("v")}}
	ts := multiTS("team-a", "x", "s", map[string]string{"k": "team-a/x"})
	if err := enforced(kube, tuck).reconcile(context.Background(), ts); err == nil {
		t.Fatal("expected error")
	}
	if len(tuck.fetched) != 0 {
		t.Error("must not fetch when the namespace cannot be read")
	}
}

func TestEnforce_BadAnnotationRejected(t *testing.T) {
	kube := &mockKube{nsAnnotations: map[string]map[string]string{"team-a": {AnnotationAllowedPaths: "../x"}}}
	tuck := &mockTuck{secrets: map[string][]byte{"team-a/x": []byte("v")}}
	ts := multiTS("team-a", "x", "s", map[string]string{"k": "team-a/x"})
	if err := enforced(kube, tuck).reconcile(context.Background(), ts); err == nil {
		t.Fatal("malformed annotation must fail closed")
	}
}

func TestNoEnforce_KeepsLegacyBehaviour(t *testing.T) {
	kube := &mockKube{nsErr: errors.New("must not be called")}
	tuck := &mockTuck{secrets: map[string][]byte{"anything/at/all": []byte("v")}}
	ts := newTS("team-a", "x", "anything/at/all", "s", "k")
	if err := New(kube, tuck, "").reconcile(context.Background(), ts); err != nil {
		t.Fatal(err)
	}
}

func TestAddTracked_DataChangeIsASpecChange(t *testing.T) {
	ctrl := New(&mockKube{}, &mockTuck{}, "")
	a := multiTS("ns", "x", "s", map[string]string{"k": "a"})
	if !ctrl.addTracked(a) {
		t.Fatal("first sight must count as changed")
	}
	if ctrl.addTracked(multiTS("ns", "x", "s", map[string]string{"k": "a"})) {
		t.Error("identical spec must not count as changed")
	}
	if !ctrl.addTracked(multiTS("ns", "x", "s", map[string]string{"k": "b"})) {
		t.Error("changed data must count as changed")
	}
}
