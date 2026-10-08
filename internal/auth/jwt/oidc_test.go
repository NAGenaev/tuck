package jwt

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/NAGenaev/tuck/internal/physical"
)

func oidcCfg() Config {
	return Config{
		JWKSURI:              "https://idp.example.com/jwks",
		Issuer:               "https://idp.example.com",
		OIDCAuthorizationURL: "https://idp.example.com/authorize",
		OIDCTokenURL:         "https://idp.example.com/token",
		OIDCClientID:         "tuck",
		OIDCRedirectURI:      "https://tuck.example.com/ui/auth/oidc/callback",
	}
}

func TestBoundGroups(t *testing.T) {
	priv, jwksURL := testKey(t)
	p := NewProvider(Config{JWKSURI: jwksURL})
	roles := []*Role{{Name: "devs", BoundGroups: []string{"team-a", "team-b"}, Policies: []string{"rw"}, TTL: time.Hour}}

	login := func(claims gojwt.MapClaims) error {
		claims["sub"] = "u1"
		claims["exp"] = time.Now().Add(time.Hour).Unix()
		_, err := p.Login(context.Background(), signToken(t, priv, claims), roles)
		return err
	}

	if err := login(gojwt.MapClaims{"groups": []string{"x", "team-b"}}); err != nil {
		t.Fatalf("matching group: %v", err)
	}
	if err := login(gojwt.MapClaims{"groups": "team-a"}); err != nil {
		t.Fatalf("string groups claim: %v", err)
	}
	if err := login(gojwt.MapClaims{"groups": []string{"x", "y"}}); !errors.Is(err, ErrNoRole) {
		t.Fatalf("non-matching group: got %v, want ErrNoRole", err)
	}
	if err := login(gojwt.MapClaims{}); !errors.Is(err, ErrNoRole) {
		t.Fatalf("no groups claim: got %v, want ErrNoRole", err)
	}
}

func TestValidateOIDC(t *testing.T) {
	if err := (&Config{JWKSURI: "x"}).ValidateOIDC(); err != nil {
		t.Fatalf("no oidc fields must be valid: %v", err)
	}
	good := oidcCfg()
	if err := good.ValidateOIDC(); err != nil {
		t.Fatalf("good config: %v", err)
	}
	if !good.OIDCEnabled() {
		t.Fatal("expected enabled")
	}

	mutate := map[string]func(*Config){
		"missing token url": func(c *Config) { c.OIDCTokenURL = "" },
		"missing client id": func(c *Config) { c.OIDCClientID = "" },
		"missing redirect":  func(c *Config) { c.OIDCRedirectURI = "" },
		"missing issuer":    func(c *Config) { c.Issuer = "" },
		"plain http authz":  func(c *Config) { c.OIDCAuthorizationURL = "http://idp.example.com/authorize" },
		"plain http token":  func(c *Config) { c.OIDCTokenURL = "http://idp.example.com/token" },
		"creds in url":      func(c *Config) { c.OIDCTokenURL = "https://u:p@idp.example.com/token" },
		"relative url":      func(c *Config) { c.OIDCRedirectURI = "/ui/cb" },
		"secret only":       func(c *Config) { *c = Config{OIDCClientSecret: "s"} },
	}
	for name, fn := range mutate {
		c := oidcCfg()
		fn(&c)
		if err := c.ValidateOIDC(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}

	local := oidcCfg()
	local.OIDCAuthorizationURL = "http://localhost:5556/auth"
	local.OIDCTokenURL = "http://127.0.0.1:5556/token"
	if err := local.ValidateOIDC(); err != nil {
		t.Fatalf("loopback http must be allowed: %v", err)
	}
}

func TestNewOIDCAttempt(t *testing.T) {
	cfg := oidcCfg()
	cfg.OIDCScopes = "groups"
	authURL, state, st, err := cfg.NewOIDCAttempt()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("state") != state || q.Get("response_type") != "code" || q.Get("client_id") != "tuck" ||
		q.Get("redirect_uri") != cfg.OIDCRedirectURI || q.Get("nonce") != st.Nonce ||
		q.Get("code_challenge_method") != "S256" {
		t.Fatalf("unexpected auth url params: %v", q)
	}
	if !strings.Contains(" "+q.Get("scope")+" ", " openid ") || !strings.Contains(q.Get("scope"), "groups") {
		t.Fatalf("scope %q must contain openid and groups", q.Get("scope"))
	}
	sum := sha256.Sum256([]byte(st.Verifier))
	if q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(sum[:]) {
		t.Fatal("code_challenge is not S256(verifier)")
	}
	_, state2, _, _ := cfg.NewOIDCAttempt()
	if state == state2 {
		t.Fatal("state must be random")
	}

	disabled := Config{}
	if _, _, _, err := disabled.NewOIDCAttempt(); !errors.Is(err, ErrOIDCNotConfigured) {
		t.Fatalf("got %v, want ErrOIDCNotConfigured", err)
	}
}

func TestLoginIDTokenNonce(t *testing.T) {
	priv, jwksURL := testKey(t)
	cfg := oidcCfg()
	cfg.JWKSURI = jwksURL
	cfg.Issuer = "https://idp.example.com"
	p := NewOIDCProvider(cfg)
	roles := []*Role{{Name: "all", Policies: []string{"p"}, TTL: time.Hour}}

	mk := func(extra gojwt.MapClaims) string {
		c := gojwt.MapClaims{
			"iss": cfg.Issuer, "sub": "u1", "aud": "tuck", "nonce": "n1",
			"exp": time.Now().Add(time.Hour).Unix(),
		}
		for k, v := range extra {
			c[k] = v
		}
		return signToken(t, priv, c)
	}
	ctx := context.Background()

	if _, err := p.LoginIDToken(ctx, mk(nil), "n1", roles); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if _, err := p.LoginIDToken(ctx, mk(nil), "other", roles); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong nonce: %v", err)
	}
	if _, err := p.LoginIDToken(ctx, mk(gojwt.MapClaims{"nonce": nil}), "n1", roles); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("missing nonce: %v", err)
	}
	if _, err := p.LoginIDToken(ctx, mk(nil), "", roles); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("empty expected nonce: %v", err)
	}
	if _, err := p.LoginIDToken(ctx, mk(gojwt.MapClaims{"aud": "someone-else"}), "n1", roles); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong audience: %v", err)
	}
}

func TestOIDCStateStore(t *testing.T) {
	s := NewStore(physical.NewInMem())
	ctx := context.Background()
	st := &OIDCState{Nonce: "n", Verifier: "v", ExpiresAt: time.Now().Add(time.Minute)}
	if err := s.PutOIDCState(ctx, "abc", st); err != nil {
		t.Fatal(err)
	}
	got, err := s.TakeOIDCState(ctx, "abc")
	if err != nil || got.Nonce != "n" || got.Verifier != "v" {
		t.Fatalf("take: %v %+v", err, got)
	}
	if _, err := s.TakeOIDCState(ctx, "abc"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("replay: %v", err)
	}
	if _, err := s.TakeOIDCState(ctx, "nope"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := s.TakeOIDCState(ctx, ""); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("empty: %v", err)
	}

	expired := &OIDCState{Nonce: "n", Verifier: "v", ExpiresAt: time.Now().Add(-time.Second)}
	_ = s.PutOIDCState(ctx, "old", expired)
	if _, err := s.TakeOIDCState(ctx, "old"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expired: %v", err)
	}
}

func TestOIDCStateSweepAndCap(t *testing.T) {
	s := NewStore(physical.NewInMem())
	ctx := context.Background()
	for i := 0; i < oidcSweepAbove; i++ {
		id := string(rune('a'+i%26)) + strings.Repeat("x", i)
		_ = s.PutOIDCState(ctx, id, &OIDCState{ExpiresAt: time.Now().Add(-time.Minute)})
	}
	// Expired records are swept on the next put, so this must succeed.
	if err := s.PutOIDCState(ctx, "fresh", &OIDCState{ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatalf("put after expiry: %v", err)
	}
	keys, _ := s.b.List(ctx, oidcStatePrefix)
	if len(keys) != 1 {
		t.Fatalf("expired states not swept: %d keys left", len(keys))
	}

	for i := 0; i < maxOIDCStates; i++ {
		_ = s.b.Put(ctx, &physical.Entry{Key: oidcStatePrefix + "k" + strings.Repeat("0", 3) + string(rune(i)) + "-" + time.Now().String(), Value: []byte(`{"expires_at":"2999-01-01T00:00:00Z"}`)})
	}
	if err := s.PutOIDCState(ctx, "overflow", &OIDCState{ExpiresAt: time.Now().Add(time.Minute)}); !errors.Is(err, ErrTooManyPending) {
		t.Fatalf("cap: %v", err)
	}
}

func TestExchangeCode(t *testing.T) {
	var gotForm url.Values
	var gotUser, gotPass string
	var basic bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		gotUser, gotPass, basic = r.BasicAuth()
		switch r.PostForm.Get("code") {
		case "good":
			_, _ = w.Write([]byte(`{"id_token":"IDT","access_token":"x"}`))
		case "noid":
			_, _ = w.Write([]byte(`{"access_token":"x"}`))
		case "redir":
			http.Redirect(w, r, "http://127.0.0.1:1/steal", http.StatusFound)
		case "boom":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		}
	}))
	defer srv.Close()

	cfg := oidcCfg()
	cfg.OIDCTokenURL = srv.URL
	cfg.OIDCClientSecret = "s3cret"
	p := NewProvider(cfg)
	ctx := context.Background()

	idt, err := p.ExchangeCode(ctx, "good", "VER")
	if err != nil || idt != "IDT" {
		t.Fatalf("good: %q %v", idt, err)
	}
	if gotForm.Get("code_verifier") != "VER" || gotForm.Get("grant_type") != "authorization_code" ||
		gotForm.Get("redirect_uri") != cfg.OIDCRedirectURI {
		t.Fatalf("form: %v", gotForm)
	}
	if !basic || gotUser != "tuck" || gotPass != "s3cret" {
		t.Fatalf("basic auth: %v %q %q", basic, gotUser, gotPass)
	}

	if _, err := p.ExchangeCode(ctx, "bad", "VER"); !errors.Is(err, ErrCodeExchange) {
		t.Fatalf("bad: %v", err)
	}
	if _, err := p.ExchangeCode(ctx, "noid", "VER"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("noid: %v", err)
	}
	if _, err := p.ExchangeCode(ctx, "boom", "VER"); !errors.Is(err, ErrIdPUnavailable) {
		t.Fatalf("boom: %v", err)
	}
	if _, err := p.ExchangeCode(ctx, "redir", "VER"); err == nil {
		t.Fatal("redirect must not be followed or accepted")
	}

	cfg.OIDCClientSecret = ""
	pub := NewProvider(cfg)
	if _, err := pub.ExchangeCode(ctx, "good", "VER"); err != nil {
		t.Fatal(err)
	}
	if basic {
		t.Fatal("public client must not send basic auth")
	}
}
