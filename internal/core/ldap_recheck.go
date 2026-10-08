package core

import (
	"context"
	"errors"
	"log"
	"sort"
	"strings"
	"time"

	authlda "github.com/NAGenaev/tuck/internal/auth/ldap"
	"github.com/NAGenaev/tuck/internal/namespace"
	"github.com/NAGenaev/tuck/internal/token"
)

// An LDAP token is a snapshot of the person's group membership at login time.
// Without a re-check, someone removed from a group keeps access until the token
// expires. RecheckLDAPTokens asks the directory again for every live LDAP
// token and revokes (or narrows) the ones that no longer match a role.

// LDAPRecheckResult summarises one re-validation pass.
type LDAPRecheckResult struct {
	// Skipped is set (and nothing was checked) when the pass did not run:
	// not the Raft leader, LDAP not configured, or checks disabled.
	Skipped      string   `json:"skipped,omitempty"`
	Users        int      `json:"users"`         // distinct people with live tokens
	Tokens       int      `json:"tokens"`        // live LDAP tokens examined
	Revoked      int      `json:"revoked"`       // tokens revoked
	Narrowed     int      `json:"narrowed"`      // tokens that lost some policies
	Errors       int      `json:"errors"`        // people the directory could not answer for
	RevokedUsers []string `json:"revoked_users,omitempty"`
}

const ldapTokenPrefix = "ldap:"

// recheckPassTimeout bounds one whole pass over all live LDAP tokens.
const recheckPassTimeout = 10 * time.Minute

// RecheckLDAPTokens re-validates every live LDAP token against the directory.
// A directory that cannot be reached never revokes anything: the token TTL
// stays the upper bound.
func (c *Core) RecheckLDAPTokens(ctx context.Context) (*LDAPRecheckResult, error) {
	// A directory that accepts connections and then stops answering must not
	// freeze the periodic pass (and with it every later revocation) forever.
	ctx, cancel := context.WithTimeout(ctx, recheckPassTimeout)
	defer cancel()
	res := &LDAPRecheckResult{}
	if cb := c.ClusterBackend(); cb != nil && !cb.IsLeader() {
		res.Skipped = "not the Raft leader"
		return res, nil
	}
	cfg, err := c.ldapStore.GetConfig(ctx)
	if err != nil {
		if errors.Is(err, authlda.ErrNotConfigured) {
			res.Skipped = "LDAP is not configured"
			return res, nil
		}
		return nil, err
	}
	roles, err := c.ldapStore.AllRoles(ctx)
	if err != nil {
		return nil, err
	}
	var opts []func(*authlda.Authenticator)
	if c.ldapDialerFn != nil {
		opts = append(opts, authlda.WithDialer(c.ldapDialerFn))
	}
	auth := authlda.NewAuthenticator(*cfg, opts...)

	ids, err := c.tokens.List(ctx)
	if err != nil {
		return nil, err
	}
	byUser := map[string][]*token.Token{}
	for _, id := range ids {
		tok, err := c.tokens.Get(ctx, id)
		if err != nil || tok == nil || tok.IsExpired() || !isLDAPToken(tok) {
			continue
		}
		u := strings.TrimPrefix(tok.DisplayName, ldapTokenPrefix)
		byUser[u] = append(byUser[u], tok)
		res.Tokens++
	}
	users := make([]string, 0, len(byUser))
	for u := range byUser {
		users = append(users, u)
	}
	sort.Strings(users)
	res.Users = len(users)

	// One directory connection for the whole pass; reopened once if it breaks.
	var sess *authlda.Session
	defer func() {
		if sess != nil {
			sess.Close()
		}
	}()
	resolve := func(u, ns string) ([]string, error) {
		for attempt := 0; ; attempt++ {
			if sess == nil {
				var oerr error
				if sess, oerr = auth.OpenSession(ctx); oerr != nil {
					return nil, oerr
				}
			}
			pol, rerr := sess.Resolve(ctx, roles, u, ns)
			if rerr != nil && errors.Is(rerr, authlda.ErrUnavailable) && attempt == 0 {
				sess.Close()
				sess = nil
				continue
			}
			return pol, rerr
		}
	}

	for _, u := range users {
		if ctx.Err() != nil {
			break
		}
		perNS := map[string][]string{}
		gone := false
		failed := false
		for _, tok := range byUser[u] {
			ns := tok.Namespace
			if ns == namespace.RootNamespace {
				ns = ""
			}
			pol, seen := perNS[ns]
			if !seen && !gone && !failed {
				pol, err = resolve(u, ns)
				switch {
				case errors.Is(err, authlda.ErrUserNotFound):
					gone, pol = true, nil
				case err != nil:
					failed = true
					res.Errors++
					log.Printf("tuck: ldap recheck: %q: %v", u, err)
				}
				perNS[ns] = pol
			}
			if failed {
				break
			}
			keep := intersect(tok.Policies, pol)
			switch {
			case len(keep) == 0:
				if err := c.RevokeTokenTree(ctx, tok.ID); err == nil {
					res.Revoked++
					if n := len(res.RevokedUsers); n == 0 || res.RevokedUsers[n-1] != u {
						res.RevokedUsers = append(res.RevokedUsers, u)
					}
				}
			case len(keep) < len(tok.Policies):
				if c.narrowToken(ctx, tok.ID, pol) {
					res.Narrowed++
				}
			}
		}
	}
	if res.Revoked > 0 || res.Narrowed > 0 {
		log.Printf("tuck: ldap recheck: revoked %d token(s) of %v, narrowed %d (users checked: %d)", res.Revoked, res.RevokedUsers, res.Narrowed, res.Users)
	}
	return res, nil
}

// RevokeLDAPUserTokens revokes every live token issued to an LDAP login (and
// the tokens those created). It is the immediate counterpart of the periodic
// re-check, for when an administrator must cut access now.
func (c *Core) RevokeLDAPUserTokens(ctx context.Context, username string) (int, error) {
	want := ldapTokenPrefix + username
	ids, err := c.tokens.List(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		tok, err := c.tokens.Get(ctx, id)
		if err != nil || tok == nil || !strings.EqualFold(tok.DisplayName, want) || !isLDAPToken(tok) {
			continue
		}
		if err := c.RevokeTokenTree(ctx, tok.ID); err == nil {
			n++
		}
	}
	return n, nil
}

// StartLDAPRecheck runs RecheckLDAPTokens in the background at the interval in
// the LDAP config (default 5m). It returns when ctx is cancelled.
func (c *Core) StartLDAPRecheck(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		var last time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if c.barrier.IsSealed() {
				continue
			}
			cfg, err := c.ldapStore.GetConfig(ctx)
			if err != nil {
				continue
			}
			every, err := cfg.RecheckEvery()
			if err != nil || every <= 0 || time.Since(last) < every {
				continue
			}
			last = time.Now()
			if _, err := c.RecheckLDAPTokens(ctx); err != nil {
				log.Printf("tuck: ldap recheck failed: %v", err)
			}
		}
	}()
}

// narrowToken reduces a token's policies to those still allowed. It re-reads
// the token right before writing, so a renewal, a use count or a revocation
// that happened during the slow directory lookups is not overwritten with a
// stale copy; a token that is gone or expired is left alone.
func (c *Core) narrowToken(ctx context.Context, id string, allowed []string) bool {
	fresh, err := c.tokens.Get(ctx, id)
	if err != nil || fresh == nil || fresh.IsExpired() {
		return false
	}
	keep := intersect(fresh.Policies, allowed)
	if len(keep) == 0 || len(keep) == len(fresh.Policies) {
		return false
	}
	fresh.Policies = keep
	return c.tokens.Put(ctx, fresh) == nil
}

// isLDAPToken reports whether tok was issued by an LDAP login. A hand-made
// token that merely carries an "ldap:" display name is not touched.
func isLDAPToken(tok *token.Token) bool {
	return strings.HasPrefix(tok.DisplayName, ldapTokenPrefix) && (tok.AuthMethod == "ldap" || tok.EntityID != "")
}

// intersect returns the elements of have that are also in allowed, in order.
func intersect(have, allowed []string) []string {
	ok := make(map[string]bool, len(allowed))
	for _, p := range allowed {
		ok[p] = true
	}
	var out []string
	for _, p := range have {
		if ok[p] {
			out = append(out, p)
		}
	}
	return out
}
