package clientip

import (
	"net/http"
	"testing"
)

func req(remote, xff string) *http.Request {
	r, _ := http.NewRequest(http.MethodGet, "http://x/", nil)
	r.RemoteAddr = remote
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return r
}

func TestFromRequest(t *testing.T) {
	t.Cleanup(func() { _ = SetTrusted(nil) })

	// No trusted proxies: the header is client input and is ignored.
	_ = SetTrusted(nil)
	if got := FromRequest(req("10.1.1.5:4000", "1.2.3.4")); got != "10.1.1.5" {
		t.Errorf("untrusted peer: got %q, want the peer address", got)
	}

	if err := SetTrusted([]string{"172.30.0.10", "10.9.0.0/16"}); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, remote, xff, want string }{
		{"trusted proxy, real client", "172.30.0.10:5555", "203.0.113.7", "203.0.113.7"},
		{"forged leftmost entry is skipped", "172.30.0.10:5555", "6.6.6.6, 203.0.113.7", "203.0.113.7"},
		{"chain of trusted hops", "172.30.0.10:5555", "203.0.113.7, 10.9.1.1", "203.0.113.7"},
		{"no header", "172.30.0.10:5555", "", "172.30.0.10"},
		{"malformed header", "172.30.0.10:5555", "not-an-ip", "172.30.0.10"},
		{"direct client forging the header", "198.51.100.9:1", "203.0.113.7", "198.51.100.9"},
	}
	for _, c := range cases {
		if got := FromRequest(req(c.remote, c.xff)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if err := SetTrusted([]string{"nonsense"}); err == nil {
		t.Error("invalid entry must be rejected")
	}
}

// Some balancers append the client port or bracket IPv6 addresses.
func TestFromRequestHopFormats(t *testing.T) {
	t.Cleanup(func() { _ = SetTrusted(nil) })
	_ = SetTrusted([]string{"172.30.0.10"})
	for _, c := range []struct{ xff, want string }{
		{"203.0.113.7:51234", "203.0.113.7"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"[2001:db8::1]", "2001:db8::1"},
		{"2001:db8::1", "2001:db8::1"},
		{"203.0.113.7:51234, 172.30.0.10:80", "203.0.113.7"},
	} {
		if got := FromRequest(req("172.30.0.10:5555", c.xff)); got != c.want {
			t.Errorf("X-Forwarded-For %q: got %q, want %q", c.xff, got, c.want)
		}
	}
}
