package api

import (
	"net"
	"strings"
	"sync"
	"time"
)

const (
	ldapMaxFailures = 5
	ldapFailWindow  = 5 * time.Minute
	ldapMaxTracked  = 10000
)

// loginThrottle counts failed LDAP logins per (username, client address) so that
// guessing passwords through Tuck cannot lock the user out of the directory,
// while one attacker cannot lock the user out for everyone else. Behind a proxy
// every client shares the proxy's address and the limit degrades to per-username.
// The zero value is ready to use.
type loginThrottle struct {
	mu    sync.Mutex
	fails map[string]*failState
}

type failState struct {
	count int
	first time.Time
}

// ldapThrottleID builds the throttle identity from the TCP peer address only;
// forwarded-for headers are client-controlled and would let an attacker dodge the limit.
func ldapThrottleID(username, remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return username + "|" + host
}

func throttleKey(username string) string { return strings.ToLower(strings.TrimSpace(username)) }

// blocked reports whether the username is locked and for how much longer.
func (t *loginThrottle) blocked(username string, now time.Time) (time.Duration, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.fails[throttleKey(username)]
	if st == nil {
		return 0, false
	}
	left := ldapFailWindow - now.Sub(st.first)
	if left <= 0 {
		delete(t.fails, throttleKey(username))
		return 0, false
	}
	return left, st.count >= ldapMaxFailures
}

func (t *loginThrottle) fail(username string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.fails == nil {
		t.fails = make(map[string]*failState)
	}
	k := throttleKey(username)
	st := t.fails[k]
	if st == nil || now.Sub(st.first) >= ldapFailWindow {
		if st == nil && len(t.fails) >= ldapMaxTracked {
			for key, s := range t.fails {
				if now.Sub(s.first) >= ldapFailWindow {
					delete(t.fails, key)
				}
			}
			if len(t.fails) >= ldapMaxTracked {
				return
			}
		}
		st = &failState{first: now}
		t.fails[k] = st
	}
	st.count++
}

func (t *loginThrottle) success(username string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fails, throttleKey(username))
}
