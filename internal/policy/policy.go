// Package policy implements Tuck's path-based ACL.
package policy

import (
	"encoding/json"
	"strings"
	"time"
)

type Capability int

const (
	CapRead   Capability = 1 << iota // GET
	CapWrite                         // PUT
	CapDelete                        // DELETE
	CapList                          // LIST
	CapDeny                          // explicit deny; overrides any allow
)

// Rule binds a glob path pattern to a set of allowed capabilities and optional
// sentinel controls (parameter filtering, wrapping TTL, MFA requirements).
type Rule struct {
	Path         string     `json:"path"`
	Capabilities Capability `json:"capabilities"`

	// Sentinel fields (all optional):

	// MinWrappingTTL / MaxWrappingTTL — when non-zero, the response for this
	// path MUST be wrapped within these TTL bounds. The server enforces this
	// on write-path operations; enforcement is advisory for reads.
	MinWrappingTTL time.Duration `json:"min_wrapping_ttl,omitempty"`
	MaxWrappingTTL time.Duration `json:"max_wrapping_ttl,omitempty"`

	// RequiredParameters lists parameter keys that MUST be present in the
	// request body. Missing keys cause a 400 before the operation proceeds.
	RequiredParameters []string `json:"required_parameters,omitempty"`

	// AllowedParameters, when non-empty, restricts the request body to only
	// the listed keys. Keys not in this list are stripped from the request.
	// Takes precedence over DeniedParameters when both are set.
	AllowedParameters []string `json:"allowed_parameters,omitempty"`

	// DeniedParameters lists parameter keys that MUST NOT appear in the
	// request body. Presence of a denied key causes a 400.
	DeniedParameters []string `json:"denied_parameters,omitempty"`

	// MFAMethods lists the MFA method names (e.g. "totp") that must be
	// satisfied before this path is accessible. An empty list means no MFA.
	MFAMethods []string `json:"mfa_methods,omitempty"`
}

// Policy is a named collection of rules.
type Policy struct {
	Name  string `json:"name"`
	Rules []Rule `json:"rules"`
	// Inheritable marks this policy as available to child namespaces.
	// When a token in a child namespace references this policy by name and no
	// local override exists, the root namespace's definition is used.
	Inheritable bool `json:"inheritable,omitempty"`
}

// RootPolicy grants full access to every path.
var RootPolicy = Policy{
	Name: "root",
	Rules: []Rule{
		{Path: "**", Capabilities: CapRead | CapWrite | CapDelete | CapList},
	},
}

// Allowed returns true if cap is permitted on path by the given policies.
// Deny rules take precedence: if any matching rule carries CapDeny the request
// is rejected regardless of any allow rules elsewhere.
func Allowed(policies []Policy, path string, cap Capability) bool {
	for _, p := range policies {
		for _, r := range p.Rules {
			if matchGlob(r.Path, path) && r.Capabilities&CapDeny != 0 {
				return false
			}
		}
	}
	for _, p := range policies {
		for _, r := range p.Rules {
			if matchGlob(r.Path, path) && r.Capabilities&cap != 0 {
				return true
			}
		}
	}
	return false
}

// matchGlob matches path against a simple glob pattern where:
//   - "**" matches any sequence of characters including "/"
//   - "*"  matches any sequence of characters that does not include "/"
func matchGlob(pattern, path string) bool {
	return globMatch(pattern, path)
}

// globMatch is a recursive descent glob matcher without external dependencies.
func globMatch(pat, s string) bool {
	for len(pat) > 0 {
		if pat == "**" {
			return true
		}
		if strings.HasPrefix(pat, "**/") {
			rest := pat[3:]
			// try matching rest against every suffix of s starting after "/"
			if globMatch(rest, s) {
				return true
			}
			for i, c := range s {
				if c == '/' {
					if globMatch(rest, s[i+1:]) {
						return true
					}
				}
			}
			return false
		}
		if pat[0] == '*' {
			// single-segment wildcard: does not cross '/'
			rest := pat[1:]
			for i := 0; i <= len(s); i++ {
				if i < len(s) && s[i] == '/' {
					break
				}
				if globMatch(rest, s[i:]) {
					return true
				}
			}
			return false
		}
		if len(s) == 0 || pat[0] != s[0] {
			return false
		}
		pat, s = pat[1:], s[1:]
	}
	return len(s) == 0
}

// MatchingRules returns all rules from the given policies whose path matches p.
func MatchingRules(policies []Policy, path string) []Rule {
	var out []Rule
	for _, pol := range policies {
		for _, r := range pol.Rules {
			if matchGlob(r.Path, path) {
				out = append(out, r)
			}
		}
	}
	return out
}

// CheckRequiredParameters returns the first required parameter that is missing
// from params. Returns "" if all required parameters are present.
func CheckRequiredParameters(rules []Rule, params map[string]any) string {
	seen := map[string]bool{}
	for _, r := range rules {
		for _, req := range r.RequiredParameters {
			if seen[req] {
				continue
			}
			seen[req] = true
			if _, ok := params[req]; !ok {
				return req
			}
		}
	}
	return ""
}

// CheckDeniedParameters returns the first denied parameter found in params.
// Returns "" if no denied parameters are present.
func CheckDeniedParameters(rules []Rule, params map[string]any) string {
	denied := map[string]bool{}
	for _, r := range rules {
		for _, d := range r.DeniedParameters {
			denied[d] = true
		}
	}
	for _, r := range rules {
		if len(r.AllowedParameters) > 0 {
			// AllowedParameters overrides DeniedParameters for this rule.
			allowed := map[string]bool{}
			for _, a := range r.AllowedParameters {
				allowed[a] = true
			}
			for k := range params {
				if !allowed[k] {
					return k // k is not in the allowed list → treat as denied
				}
			}
			return ""
		}
	}
	for k := range params {
		if denied[k] {
			return k
		}
	}
	return ""
}

// FilterParameters removes keys not in the AllowedParameters list.
// If no rule specifies AllowedParameters, params is returned unchanged.
func FilterParameters(rules []Rule, params map[string]any) map[string]any {
	var allowed map[string]bool
	for _, r := range rules {
		if len(r.AllowedParameters) > 0 {
			if allowed == nil {
				allowed = map[string]bool{}
			}
			for _, a := range r.AllowedParameters {
				allowed[a] = true
			}
		}
	}
	if allowed == nil {
		return params
	}
	out := make(map[string]any, len(allowed))
	for k, v := range params {
		if allowed[k] {
			out[k] = v
		}
	}
	return out
}

// RequiredMFAMethods returns the union of all MFA methods required by matching rules.
func RequiredMFAMethods(rules []Rule) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rules {
		for _, m := range r.MFAMethods {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out
}

// WrappingBounds returns the tightest wrapping TTL bounds across all matching rules.
// Returns (0,0) if no wrapping bounds are set.
func WrappingBounds(rules []Rule) (min, max time.Duration) {
	for _, r := range rules {
		if r.MinWrappingTTL > 0 && (min == 0 || r.MinWrappingTTL > min) {
			min = r.MinWrappingTTL
		}
		if r.MaxWrappingTTL > 0 && (max == 0 || r.MaxWrappingTTL < max) {
			max = r.MaxWrappingTTL
		}
	}
	return
}

func (p *Policy) marshal() ([]byte, error)    { return json.Marshal(p) }
func unmarshal(data []byte) (*Policy, error) { var p Policy; return &p, json.Unmarshal(data, &p) }
