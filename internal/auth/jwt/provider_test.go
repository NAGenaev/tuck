package jwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// testKey generates a fresh RSA key pair and returns (private, JWKS server URL).
func testKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	pub := &priv.PublicKey
	nB64 := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	eB64 := base64.RawURLEncoding.EncodeToString(eBytes)

	jwksBody, _ := json.Marshal(map[string]interface{}{
		"keys": []map[string]interface{}{{
			"kty": "RSA",
			"kid": "test-key",
			"use": "sig",
			"alg": "RS256",
			"n":   nB64,
			"e":   eB64,
		}},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jwksBody)
	}))
	t.Cleanup(srv.Close)
	return priv, srv.URL
}

func signToken(t *testing.T, priv *rsa.PrivateKey, claims gojwt.MapClaims) string {
	t.Helper()
	tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "test-key"
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLoginSuccess(t *testing.T) {
	priv, jwksURL := testKey(t)
	p := NewProvider(Config{JWKSURI: jwksURL, Issuer: "https://issuer.test"})

	roles := []*Role{{
		Name:         "admin",
		BoundSubject: "user1",
		Policies:     []string{"admin"},
		TTL:          30 * time.Minute,
	}}

	tok := signToken(t, priv, gojwt.MapClaims{
		"iss": "https://issuer.test",
		"sub": "user1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	result, err := p.Login(context.Background(), tok, roles)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.Subject != "user1" {
		t.Fatalf("expected subject user1, got %q", result.Subject)
	}
	if len(result.Policies) != 1 || result.Policies[0] != "admin" {
		t.Fatalf("unexpected policies: %v", result.Policies)
	}
}

func TestLoginWrongIssuer(t *testing.T) {
	priv, jwksURL := testKey(t)
	p := NewProvider(Config{JWKSURI: jwksURL, Issuer: "https://expected.test"})

	tok := signToken(t, priv, gojwt.MapClaims{
		"iss": "https://other.test",
		"sub": "user1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	_, err := p.Login(context.Background(), tok, []*Role{{Name: "r", Policies: []string{"x"}}})
	if err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestLoginExpiredToken(t *testing.T) {
	priv, jwksURL := testKey(t)
	p := NewProvider(Config{JWKSURI: jwksURL})

	tok := signToken(t, priv, gojwt.MapClaims{
		"sub": "user1",
		"exp": time.Now().Add(-time.Hour).Unix(), // already expired
	})

	_, err := p.Login(context.Background(), tok, []*Role{{Name: "r", Policies: []string{"x"}}})
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestLoginNoMatchingRole(t *testing.T) {
	priv, jwksURL := testKey(t)
	p := NewProvider(Config{JWKSURI: jwksURL})

	roles := []*Role{{
		Name:         "admin",
		BoundSubject: "admin-user",
		Policies:     []string{"admin"},
	}}

	tok := signToken(t, priv, gojwt.MapClaims{
		"sub": "regular-user",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	_, err := p.Login(context.Background(), tok, roles)
	if err != ErrNoRole {
		t.Fatalf("expected ErrNoRole, got %v", err)
	}
}

func TestLoginBoundClaims(t *testing.T) {
	priv, jwksURL := testKey(t)
	p := NewProvider(Config{JWKSURI: jwksURL})

	roles := []*Role{{
		Name:        "tenant-admin",
		BoundClaims: map[string]string{"tenant": "acme", "role": "admin"},
		Policies:    []string{"acme-admin"},
		TTL:         time.Hour,
	}}

	// Token with matching claims.
	tok := signToken(t, priv, gojwt.MapClaims{
		"sub":    "u1",
		"tenant": "acme",
		"role":   "admin",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	result, err := p.Login(context.Background(), tok, roles)
	if err != nil {
		t.Fatalf("Login with matching claims: %v", err)
	}
	if result.Policies[0] != "acme-admin" {
		t.Fatalf("wrong policy: %v", result.Policies)
	}

	// Token with wrong tenant — should get ErrNoRole.
	tok2 := signToken(t, priv, gojwt.MapClaims{
		"sub":    "u1",
		"tenant": "other",
		"role":   "admin",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	_, err = p.Login(context.Background(), tok2, roles)
	if err != ErrNoRole {
		t.Fatalf("expected ErrNoRole for wrong tenant, got %v", err)
	}
}

func TestParseSecretsList(t *testing.T) {
	// smoke test: JWKS URL construction logic
	uri := fmt.Sprintf("https://issuer.test/.well-known/jwks.json")
	j := NewJWKS(uri, 0, nil)
	if j.uri != uri {
		t.Fatalf("unexpected URI: %q", j.uri)
	}
}
