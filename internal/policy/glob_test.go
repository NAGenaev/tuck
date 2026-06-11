package policy

import "testing"

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pat  string
		path string
		want bool
	}{
		{"**", "secret/db/pass", true},
		{"**", "anything", true},
		{"secret/**", "secret/db/pass", true},
		{"secret/**", "secret/x", true},
		{"secret/**", "other/db/pass", false},
		{"secret/db/*", "secret/db/pass", true},
		{"secret/db/*", "secret/db/a/b", false},
		{"secret/db/*", "secret/other", false},
		{"secret/exact", "secret/exact", true},
		{"secret/exact", "secret/other", false},
	}
	for _, c := range cases {
		got := globMatch(c.pat, c.path)
		if got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.pat, c.path, got, c.want)
		}
	}
}

func TestAllowedRootPolicy(t *testing.T) {
	policies := []Policy{RootPolicy}
	paths := []string{"secret/x", "secret/a/b/c", "auth/token/xyz"}
	caps := []Capability{CapRead, CapWrite, CapDelete, CapList}
	for _, p := range paths {
		for _, c := range caps {
			if !Allowed(policies, p, c) {
				t.Errorf("RootPolicy should allow all: path=%q cap=%d", p, c)
			}
		}
	}
}
