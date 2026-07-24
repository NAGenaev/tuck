package ui

import (
	"net/http/httptest"
	"testing"
)

func TestHandler_ServesIndex(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_FallsBackToIndexForSPARoute(t *testing.T) {
	req := httptest.NewRequest("GET", "/dashboard/status", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200 (index.html fallback), got %d", rec.Code)
	}

	index := httptest.NewRecorder()
	Handler().ServeHTTP(index, httptest.NewRequest("GET", "/", nil))

	if rec.Body.String() != index.Body.String() {
		t.Fatalf("SPA route body did not match index.html body")
	}
}

func TestHandler_ServesRealNestedAsset(t *testing.T) {
	req := httptest.NewRequest("GET", "/locales/en/translation.json", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
