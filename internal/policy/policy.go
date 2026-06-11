// Package policy implements Tuck's path-based ACL.
package policy

import (
	"encoding/json"
	"strings"
)

type Capability int

const (
	CapRead   Capability = 1 << iota // GET
	CapWrite                         // PUT
	CapDelete                        // DELETE
	CapList                          // LIST (future)
)

// Rule binds a glob path pattern to a set of allowed capabilities.
type Rule struct {
	Path         string     `json:"path"`
	Capabilities Capability `json:"capabilities"`
}

// Policy is a named collection of rules.
type Policy struct {
	Name  string `json:"name"`
	Rules []Rule `json:"rules"`
}

// RootPolicy grants full access to every path.
var RootPolicy = Policy{
	Name: "root",
	Rules: []Rule{
		{Path: "**", Capabilities: CapRead | CapWrite | CapDelete | CapList},
	},
}

// Allowed returns true if at least one rule in any of the given policies
// matches path and permits cap.
func Allowed(policies []Policy, path string, cap Capability) bool {
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

func (p *Policy) marshal() ([]byte, error)    { return json.Marshal(p) }
func unmarshal(data []byte) (*Policy, error) { var p Policy; return &p, json.Unmarshal(data, &p) }
