package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// generateED25519PubKey generates a fresh Ed25519 key pair and returns the
// public key in OpenSSH authorized_keys format (for use with SSH sign endpoint).
func generateED25519PubKey(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("wrap ssh public key: %v", err)
	}
	return strings.TrimSpace(string(gossh.MarshalAuthorizedKey(sshPub)))
}

// TestSSHGenerateCA verifies that CA generation returns a valid OpenSSH public key.
func TestSSHGenerateCA(t *testing.T) {
	ts, _, root := newTestServer(t)

	status, body := doJSON(t, ts, http.MethodPost, "/v1/ssh/generate/ca", "", root)
	if status != http.StatusOK {
		t.Fatalf("generate CA: %d %s", status, body)
	}
	pubKey := jsonField(t, body, "public_key")
	if pubKey == "" {
		t.Fatal("public_key is empty")
	}
	// Must be parseable as an SSH public key.
	if _, _, _, _, err := gossh.ParseAuthorizedKey([]byte(pubKey)); err != nil {
		t.Errorf("CA public key not valid SSH: %v", err)
	}
}

// TestSSHGetCAPublicKey verifies that the CA public key is readable after generation.
func TestSSHGetCAPublicKey(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPost, "/v1/ssh/generate/ca", "", root)

	status, body := doJSON(t, ts, http.MethodGet, "/v1/ssh/ca/public-key", "", root)
	if status != http.StatusOK {
		t.Fatalf("get CA public key: %d %s", status, body)
	}
	if jsonField(t, body, "public_key") == "" {
		t.Error("public_key is empty")
	}
}

// TestSSHGetCAPublicKey_NoCA verifies 404 when no CA exists.
func TestSSHGetCAPublicKey_NoCA(t *testing.T) {
	ts, _, _ := newTestServer(t)

	status, _ := doJSON(t, ts, http.MethodGet, "/v1/ssh/ca/public-key", "", "")
	if status != http.StatusNotFound {
		t.Errorf("no CA: %d, want 404", status)
	}
}

// TestSSHRoleCRUD exercises the full role management lifecycle.
func TestSSHRoleCRUD(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPost, "/v1/ssh/generate/ca", "", root)

	roleBody := `{"allowed_users":["ubuntu","ec2-user"],"cert_type":"user","default_ttl":"24h","max_ttl":"168h"}`

	// Create.
	status, body := doJSON(t, ts, http.MethodPut, "/v1/ssh/roles/web", roleBody, root)
	if status != http.StatusOK {
		t.Fatalf("put role: %d %s", status, body)
	}

	// Read.
	status, body = doJSON(t, ts, http.MethodGet, "/v1/ssh/roles/web", "", root)
	if status != http.StatusOK {
		t.Fatalf("get role: %d %s", status, body)
	}
	if !strings.Contains(string(body), "ubuntu") {
		t.Errorf("role body missing ubuntu: %s", body)
	}

	// List.
	status, body = doJSON(t, ts, http.MethodGet, "/v1/ssh/roles/?list=true", "", root)
	if status != http.StatusOK {
		t.Fatalf("list roles: %d %s", status, body)
	}
	if !strings.Contains(string(body), "web") {
		t.Errorf("list missing web: %s", body)
	}

	// Delete.
	status, _ = doJSON(t, ts, http.MethodDelete, "/v1/ssh/roles/web", "", root)
	if status != http.StatusNoContent {
		t.Errorf("delete role: %d, want 204", status)
	}

	// Must be gone.
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/ssh/roles/web", "", root)
	if status != http.StatusNotFound {
		t.Errorf("get deleted role: %d, want 404", status)
	}
}

// TestSSHSignUserCert verifies the full sign flow: generate CA → create role →
// sign a public key → validate the resulting certificate.
func TestSSHSignUserCert(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPost, "/v1/ssh/generate/ca", "", root)

	roleBody := `{"allowed_users":["ubuntu"],"cert_type":"user","default_ttl":"1h"}`
	doJSON(t, ts, http.MethodPut, "/v1/ssh/roles/web", roleBody, root)

	pubKey := generateED25519PubKey(t)
	signBody := mustJSON(map[string]any{
		"public_key":       pubKey,
		"valid_principals": []string{"ubuntu"},
		"ttl":              "1h",
	})
	status, body := doJSON(t, ts, http.MethodPost, "/v1/ssh/sign/web", signBody, root)
	if status != http.StatusOK {
		t.Fatalf("sign: %d %s", status, body)
	}

	signedKey := jsonField(t, body, "signed_key")
	if signedKey == "" {
		t.Fatal("signed_key is empty")
	}
	// Must parse as an SSH certificate.
	pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(signedKey))
	if err != nil {
		t.Fatalf("parse signed cert: %v", err)
	}
	if _, ok := pub.(*gossh.Certificate); !ok {
		t.Errorf("parsed key is %T, want *gossh.Certificate", pub)
	}
}

// TestSSHSignPrincipalDenied verifies 403 when the requested principal is not
// in allowed_users.
func TestSSHSignPrincipalDenied(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPost, "/v1/ssh/generate/ca", "", root)

	roleBody := `{"allowed_users":["ubuntu"],"cert_type":"user","default_ttl":"1h"}`
	doJSON(t, ts, http.MethodPut, "/v1/ssh/roles/web", roleBody, root)

	pubKey := generateED25519PubKey(t)
	signBody := mustJSON(map[string]any{
		"public_key":       pubKey,
		"valid_principals": []string{"root"},
	})
	status, _ := doJSON(t, ts, http.MethodPost, "/v1/ssh/sign/web", signBody, root)
	if status != http.StatusForbidden {
		t.Errorf("denied principal: %d, want 403", status)
	}
}

// TestSSHSignNoCA verifies 503 when signing is attempted without a CA.
func TestSSHSignNoCA(t *testing.T) {
	ts, _, root := newTestServer(t)

	roleBody := `{"allowed_users":["ubuntu"],"cert_type":"user","default_ttl":"1h"}`
	doJSON(t, ts, http.MethodPut, "/v1/ssh/roles/web", roleBody, root)

	pubKey := generateED25519PubKey(t)
	signBody := mustJSON(map[string]any{
		"public_key":       pubKey,
		"valid_principals": []string{"ubuntu"},
	})
	status, _ := doJSON(t, ts, http.MethodPost, "/v1/ssh/sign/web", signBody, root)
	if status != http.StatusServiceUnavailable {
		t.Errorf("no CA: %d, want 503", status)
	}
}

// TestSSHSignUnknownRole verifies 404 when the role does not exist.
func TestSSHSignUnknownRole(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPost, "/v1/ssh/generate/ca", "", root)

	pubKey := generateED25519PubKey(t)
	signBody := mustJSON(map[string]any{
		"public_key":       pubKey,
		"valid_principals": []string{"ubuntu"},
	})
	status, _ := doJSON(t, ts, http.MethodPost, "/v1/ssh/sign/ghost", signBody, root)
	if status != http.StatusNotFound {
		t.Errorf("unknown role: %d, want 404", status)
	}
}

// TestSSHSignAnyPrincipal verifies that an empty allowed_users list permits
// any principal.
func TestSSHSignAnyPrincipal(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPost, "/v1/ssh/generate/ca", "", root)

	// No allowed_users restriction.
	roleBody := `{"cert_type":"user","default_ttl":"1h"}`
	doJSON(t, ts, http.MethodPut, "/v1/ssh/roles/open", roleBody, root)

	pubKey := generateED25519PubKey(t)
	signBody := mustJSON(map[string]any{
		"public_key":       pubKey,
		"valid_principals": []string{"root"},
	})
	status, body := doJSON(t, ts, http.MethodPost, "/v1/ssh/sign/open", signBody, root)
	if status != http.StatusOK {
		t.Errorf("open role any principal: %d %s, want 200", status, body)
	}
}

// TestSSHSignMissingPublicKey verifies 400 when public_key is absent.
func TestSSHSignMissingPublicKey(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPost, "/v1/ssh/generate/ca", "", root)
	doJSON(t, ts, http.MethodPut, "/v1/ssh/roles/web",
		`{"cert_type":"user","default_ttl":"1h"}`, root)

	status, _ := doJSON(t, ts, http.MethodPost, "/v1/ssh/sign/web", `{}`, root)
	if status != http.StatusBadRequest {
		t.Errorf("missing public_key: %d, want 400", status)
	}
}

// TestSSHImportCA verifies that an externally generated CA can be imported.
func TestSSHImportCA(t *testing.T) {
	ts, _, root := newTestServer(t)

	// Generate a temporary CA to get a PEM-encoded private key to import.
	// We first generate via the API to get the public key, then we can't
	// easily get the private key from the API. Instead, generate locally
	// and import.
	_, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519: %v", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(edPriv)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	privPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}))

	importBody := mustJSON(map[string]string{"private_key": privPEM})
	status, body := doJSON(t, ts, http.MethodPost, "/v1/ssh/import/ca", importBody, root)
	if status != http.StatusOK {
		t.Fatalf("import CA: %d %s", status, body)
	}

	// CA public key must now be readable.
	status, body = doJSON(t, ts, http.MethodGet, "/v1/ssh/ca/public-key", "", root)
	if status != http.StatusOK {
		t.Fatalf("get imported CA public key: %d %s", status, body)
	}
	if jsonField(t, body, "public_key") == "" {
		t.Error("imported CA public_key is empty")
	}
}
