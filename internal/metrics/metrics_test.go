package metrics

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_2xxCounter(t *testing.T) {
	// Reset the counter before the test (package-level state is shared).
	RequestsTotal[0].Store(0)
	RequestsTotal[5].Store(0)

	const calls = 3
	for i := 0; i < calls; i++ {
		Inc2xx()
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	Handler()(rec, req)

	body := rec.Body.String()
	want := fmt.Sprintf(`tuck_requests_total{class="2xx"} %d`, calls)
	if !strings.Contains(body, want) {
		t.Errorf("expected body to contain %q, got:\n%s", want, body)
	}
}

func TestHandler_ContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	Handler()(rec, req)

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}
}
