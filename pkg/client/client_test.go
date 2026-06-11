package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tuck-Token") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/v1/secret/db/password" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"path": "db/password", "value": "s3cr3t"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token", WithHTTPClient(srv.Client()))
	val, err := c.GetSecret(context.Background(), "db/password")
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "s3cr3t" {
		t.Fatalf("expected s3cr3t, got %s", val)
	}
}

func TestGetSecretNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))
	defer srv.Close()

	c := New(srv.URL, "token", WithHTTPClient(srv.Client()))
	val, err := c.GetSecret(context.Background(), "missing/key")
	if err != nil {
		t.Fatalf("expected nil error for 404, got %v", err)
	}
	if val != nil {
		t.Fatal("expected nil value for missing key")
	}
}

func TestPutSecret(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		gotBody = buf[:n]
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "token", WithHTTPClient(srv.Client()))
	if err := c.PutSecret(context.Background(), "db/pass", []byte("hunter2")); err != nil {
		t.Fatal(err)
	}
	if string(gotBody) != "hunter2" {
		t.Fatalf("body mismatch: %s", gotBody)
	}
}

func TestSealStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"sealed": false, "type": "dev"})
	}))
	defer srv.Close()

	c := New(srv.URL, "", WithHTTPClient(srv.Client()))
	st, err := c.SealStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Sealed {
		t.Fatal("expected unsealed")
	}
	if st.Type != "dev" {
		t.Fatalf("expected dev seal, got %s", st.Type)
	}
}

func TestErrorTypes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "sealed"})
	}))
	defer srv.Close()

	c := New(srv.URL, "t", WithHTTPClient(srv.Client()))
	_, err := c.GetSecret(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsSealed(err) {
		t.Fatalf("expected IsSealed=true, got %v", err)
	}
}

func TestListSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "LIST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"keys": []string{"db/password", "db/user"}})
	}))
	defer srv.Close()

	c := New(srv.URL, "token", WithHTTPClient(srv.Client()))
	keys, err := c.ListSecrets(context.Background(), "db/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestKVv2Write(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"version": 1, "path": "db/pass"})
	}))
	defer srv.Close()

	c := New(srv.URL, "token", WithHTTPClient(srv.Client()))
	res, err := c.KVv2().Write(context.Background(), "db/pass", []byte("secret"), -1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Version != 1 {
		t.Fatalf("expected version 1, got %d", res.Version)
	}
}

func TestIsNotFound(t *testing.T) {
	err := &Error{StatusCode: 404, Message: "not found"}
	if !IsNotFound(err) {
		t.Fatal("expected IsNotFound=true")
	}
	if IsSealed(err) {
		t.Fatal("expected IsSealed=false")
	}
}
