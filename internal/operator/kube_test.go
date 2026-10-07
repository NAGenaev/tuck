package operator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestManagedDataChanged(t *testing.T) {
	cases := []struct {
		name           string
		desired, exist map[string][]byte
		wantChanged    bool
	}{
		{"new key missing from existing", map[string][]byte{"a": []byte("1")}, map[string][]byte{}, true},
		{"same value", map[string][]byte{"a": []byte("1")}, map[string][]byte{"a": []byte("1")}, false},
		{"different value", map[string][]byte{"a": []byte("1")}, map[string][]byte{"a": []byte("2")}, true},
		{"foreign key in existing ignored", map[string][]byte{"a": []byte("1")}, map[string][]byte{"a": []byte("1"), "b": []byte("x")}, false},
		{"one of several keys changed", map[string][]byte{"a": []byte("1"), "b": []byte("2")}, map[string][]byte{"a": []byte("1"), "b": []byte("9")}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := managedDataChanged(tc.desired, tc.exist); got != tc.wantChanged {
				t.Errorf("managedDataChanged(%v, %v) = %v, want %v", tc.desired, tc.exist, got, tc.wantChanged)
			}
		})
	}
}

func TestRestartWorkload_PatchShape(t *testing.T) {
	var gotMethod, gotPath, gotContentType string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	tokenFile, err := os.CreateTemp(t.TempDir(), "sa-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tokenFile.WriteString("fake-jwt"); err != nil {
		t.Fatal(err)
	}
	_ = tokenFile.Close()

	c, err := NewKubeClient(srv.URL, tokenFile.Name(), "")
	if err != nil {
		t.Fatal(err)
	}

	if err := c.RestartWorkload(context.Background(), "prod", RestartTarget{Kind: "Deployment", Name: "api"}, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("RestartWorkload: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if want := "/apis/apps/v1/namespaces/prod/deployments/api"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotContentType != "application/strategic-merge-patch+json" {
		t.Errorf("content-type = %q, want strategic-merge-patch+json", gotContentType)
	}
	spec, _ := gotBody["spec"].(map[string]any)
	tmpl, _ := spec["template"].(map[string]any)
	meta, _ := tmpl["metadata"].(map[string]any)
	ann, _ := meta["annotations"].(map[string]any)
	if ann["tuck.io/restartedAt"] != "2026-01-01T00:00:00Z" {
		t.Errorf("patch body annotations = %v, missing/wrong tuck.io/restartedAt", ann)
	}
}

func TestRestartWorkload_UnsupportedKind(t *testing.T) {
	tokenFile, err := os.CreateTemp(t.TempDir(), "sa-token")
	if err != nil {
		t.Fatal(err)
	}
	_ = tokenFile.Close()
	c, err := NewKubeClient("http://unused", tokenFile.Name(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RestartWorkload(context.Background(), "prod", RestartTarget{Kind: "Pod", Name: "x"}, "now"); err == nil {
		t.Fatal("expected an error for an unsupported restart target kind (Pod), got nil")
	}
}
