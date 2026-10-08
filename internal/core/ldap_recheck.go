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

// RecheckLDAPTokens re-validates every live LDAP token against the directory.
// A directory that cannot be reached never revokes anything: the token TTL
// stays the upper bound.
func (c *Core) RecheckLDAPTokens(ctx context.Context) (*LDAPRecheckResult, error) {
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
				pol, err = auth.Resolve(ctx, roles, u, ns)
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
				tok.Policies = keep
				if err := c.tokens.Put(ctx, tok); err == nil {
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
		if err != nil || tok == nil || !strings.EqualFold(tok.DisplayName, want) {
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
