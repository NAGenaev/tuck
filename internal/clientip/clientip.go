// Package clientip resolves the address of the real client of an HTTP request.
//
// Tuck usually runs behind a load balancer, so r.RemoteAddr is the balancer's
// address for every request. The balancer reports the real client in
// X-Forwarded-For, but that header is plain client input unless the request
// really came from the balancer. Therefore it is honoured only when the
// direct peer is inside a configured list of trusted proxies.
package clientip

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
)

var trusted atomic.Pointer[[]*net.IPNet]

// SetTrusted configures the proxies whose X-Forwarded-For is believed. Entries
// are CIDRs or single IPs. An empty list (the default) trusts no proxy, so
// X-Forwarded-For is ignored.
func SetTrusted(entries []string) error {
	var nets []*net.IPNet
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !strings.Contains(e, "/") {
			ip := net.ParseIP(e)
			if ip == nil {
				return fmt.Errorf("trusted proxy %q: not an IP or CIDR", e)
			}
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			e = fmt.Sprintf("%s/%d", ip.String(), bits)
		}
		_, n, err := net.ParseCIDR(e)
		if err != nil {
			return fmt.Errorf("trusted proxy %q: %w", e, err)
		}
		nets = append(nets, n)
	}
	trusted.Store(&nets)
	return nil
}

func isTrusted(ip net.IP) bool {
	p := trusted.Load()
	if p == nil || ip == nil {
		return false
	}
	for _, n := range *p {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// parseHop reads one X-Forwarded-For entry: a bare address, "ip:port",
// "[ipv6]:port" or "[ipv6]" (some balancers append the client port).
func parseHop(h string) net.IP {
	h = strings.TrimSpace(h)
	if ip := net.ParseIP(h); ip != nil {
		return ip
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(strings.Trim(h, "[]"))
}

// FromRequest returns the client address (host only, no port). When the direct
// peer is a trusted proxy, X-Forwarded-For is read from the right: trusted
// hops are skipped and the first untrusted address wins, so a client cannot
// forge its address by prepending entries.
func FromRequest(r *http.Request) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	if !isTrusted(net.ParseIP(peer)) {
		return peer
	}
	var hops []string
	for _, h := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(h, ",") {
			if part = strings.TrimSpace(part); part != "" {
				hops = append(hops, part)
			}
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		ip := parseHop(hops[i])
		if ip == nil {
			return peer // malformed header: do not guess
		}
		if !isTrusted(ip) {
			return ip.String()
		}
	}
	return peer
}
