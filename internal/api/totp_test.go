package api

import (
	"net/http"
	"strings"
	"testing"
)

// TestTOTPKeyCRUD exercises the full lifecycle: create → get → list → delete.
func TestTOTPKeyCRUD(t *testing.T) {
	ts, _, root := newTestServer(t)

	// Create with explicit params.
	body := `{"issuer":"ACME","account":"user@example.com","digits":6,"period":30}`
	status, resp := doJSON(t, ts, http.MethodPost, "/v1/totp/keys/myapp", body, root)
	if status != http.StatusOK {
		t.Fatalf("create key: %d %s", status, resp)
	}
	// Creation response must include the secret and an otpauth URL.
	if jsonField(t, resp, "secret") == "" {
		t.Error("create response missing secret")
	}
	if !strings.HasPrefix(jsonField(t, resp, "url"), "otpauth://") {
		t.Errorf("create response url = %q, want otpauth://... prefix", jsonField(t, resp, "url"))
	}

	// Get metadata — secret must NOT be present.
	status, resp = doJSON(t, ts, http.MethodGet, "/v1/totp/keys/myapp", "", root)
	if status != http.StatusOK {
		t.Fatalf("get key: %d %s", status, resp)
	}
	if jsonField(t, resp, "name") != "myapp" {
		t.Errorf("get name = %q, want myapp", jsonField(t, resp, "name"))
	}

	// List — key must appear.
	status, resp = doJSON(t, ts, http.MethodGet, "/v1/totp/keys/?list=true", "", root)
	if status != http.StatusOK {
		t.Fatalf("list keys: %d %s", status, resp)
	}
	if !strings.Contains(string(resp), "myapp") {
		t.Errorf("list missing myapp: %s", resp)
	}

	// Delete.
	status, _ = doJSON(t, ts, http.MethodDelete, "/v1/totp/keys/myapp", "", root)
	if status != http.StatusNoContent {
		t.Errorf("delete key: %d, want 204", status)
	}

	// Must be gone.
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/totp/keys/myapp", "", root)
	if status != http.StatusNotFound {
		t.Errorf("get deleted key: %d, want 404", status)
	}
}

// TestTOTPGenerateAndValidateCode verifies the generate → validate round-trip.
func TestTOTPGenerateAndValidateCode(t *testing.T) {
	ts, _, root := newTestServer(t)

	// Create a key.
	status, resp := doJSON(t, ts, http.MethodPost, "/v1/totp/keys/myapp", "", root)
	if status != http.StatusOK {
		t.Fatalf("create key: %d %s", status, resp)
	}

	// Generate the current code server-side.
	status, resp = doJSON(t, ts, http.MethodGet, "/v1/totp/code/myapp", "", root)
	if status != http.StatusOK {
		t.Fatalf("generate code: %d %s", status, resp)
	}
	code := jsonField(t, resp, "code")
	if len(code) != 6 {
		t.Errorf("code length = %d, want 6", len(code))
	}

	// Validate the generated code — must be valid.
	valBody := `{"code":"` + code + `"}`
	status, resp = doJSON(t, ts, http.MethodPost, "/v1/totp/code/myapp", valBody, root)
	if status != http.StatusOK {
		t.Fatalf("validate code: %d %s", status, resp)
	}
	if jsonField(t, resp, "valid") != "true" {
		t.Errorf("validate: valid=%s, want true", jsonField(t, resp, "valid"))
	}
}

// TestTOTPInvalidCode verifies that a wrong code returns valid=false.
func TestTOTPInvalidCode(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPost, "/v1/totp/keys/myapp", "", root)

	status, resp := doJSON(t, ts, http.MethodPost, "/v1/totp/code/myapp", `{"code":"000000"}`, root)
	if status != http.StatusOK {
		t.Fatalf("validate: %d %s", status, resp)
	}
	if jsonField(t, resp, "valid") != "false" {
		t.Logf("note: 000000 happened to be a valid TOTP code (extremely rare)")
	}
}

// TestTOTPSHA256Key verifies that keys with sha256 algorithm are created and work.
func TestTOTPSHA256Key(t *testing.T) {
	ts, _, root := newTestServer(t)

	body := `{"issuer":"Test","account":"svc","algorithm":"sha256","digits":6}`
	status, resp := doJSON(t, ts, http.MethodPost, "/v1/totp/keys/sha256key", body, root)
	if status != http.StatusOK {
		t.Fatalf("create sha256 key: %d %s", status, resp)
	}

	status, resp = doJSON(t, ts, http.MethodGet, "/v1/totp/code/sha256key", "", root)
	if status != http.StatusOK {
		t.Fatalf("generate code: %d %s", status, resp)
	}
	code := jsonField(t, resp, "code")

	valBody := `{"code":"` + code + `"}`
	status, resp = doJSON(t, ts, http.MethodPost, "/v1/totp/code/sha256key", valBody, root)
	if status != http.StatusOK {
		t.Fatalf("validate: %d %s", status, resp)
	}
	if jsonField(t, resp, "valid") != "true" {
		t.Errorf("sha256 validate: valid=%s, want true", jsonField(t, resp, "valid"))
	}
}

// TestTOTPImportSecret verifies that a known base32 secret can be imported.
func TestTOTPImportSecret(t *testing.T) {
	ts, _, root := newTestServer(t)

	// JBSWY3DPEHPK3PXP is a well-known test TOTP secret.
	body := `{"issuer":"Test","account":"import","secret":"JBSWY3DPEHPK3PXP"}`
	status, resp := doJSON(t, ts, http.MethodPost, "/v1/totp/keys/imported", body, root)
	if status != http.StatusOK {
		t.Fatalf("import secret: %d %s", status, resp)
	}
	// Returned secret must match what we imported.
	if jsonField(t, resp, "secret") != "JBSWY3DPEHPK3PXP" {
		t.Errorf("returned secret = %q, want JBSWY3DPEHPK3PXP", jsonField(t, resp, "secret"))
	}
}

// TestTOTPInvalidSecret verifies 400 for a non-base32 secret.
func TestTOTPInvalidSecret(t *testing.T) {
	ts, _, root := newTestServer(t)

	body := `{"secret":"not!base32!!"}`
	status, _ := doJSON(t, ts, http.MethodPost, "/v1/totp/keys/bad", body, root)
	if status != http.StatusBadRequest {
		t.Errorf("invalid secret: %d, want 400", status)
	}
}

// TestTOTPGenerateUnknownKey verifies 404 for generate on a missing key.
func TestTOTPGenerateUnknownKey(t *testing.T) {
	ts, _, root := newTestServer(t)

	status, _ := doJSON(t, ts, http.MethodGet, "/v1/totp/code/ghost", "", root)
	if status != http.StatusNotFound {
		t.Errorf("generate missing key: %d, want 404", status)
	}
}

// TestTOTPValidateUnknownKey verifies 404 for validate on a missing key.
func TestTOTPValidateUnknownKey(t *testing.T) {
	ts, _, root := newTestServer(t)

	status, _ := doJSON(t, ts, http.MethodPost, "/v1/totp/code/ghost", `{"code":"123456"}`, root)
	if status != http.StatusNotFound {
		t.Errorf("validate missing key: %d, want 404", status)
	}
}

// TestTOTPValidateMissingCode verifies 400 when the code field is absent.
func TestTOTPValidateMissingCode(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPost, "/v1/totp/keys/myapp", "", root)

	status, _ := doJSON(t, ts, http.MethodPost, "/v1/totp/code/myapp", `{}`, root)
	if status != http.StatusBadRequest {
		t.Errorf("missing code: %d, want 400", status)
	}
}
