package policy

import "testing"

// FuzzGlobMatch tests that globMatch never panics on arbitrary pattern/path inputs.
func FuzzGlobMatch(f *testing.F) {
	f.Add("secret/*", "secret/db")
	f.Add("auth/**", "auth/k8s/login")
	f.Add("**", "")
	f.Add("", "anything")
	f.Add("a/*/b", "a/x/b")
	f.Add("**/end", "a/b/c/end")
	f.Add(string([]byte{0x00, 0x2f, 0x2a}), "x/y")

	f.Fuzz(func(t *testing.T, pattern, path string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("globMatch panicked on pattern=%q path=%q: %v", pattern, path, r)
			}
		}()
		_ = globMatch(pattern, path)
	})
}

// FuzzAllowed tests that Allowed never panics on arbitrary policy/path/cap inputs.
func FuzzAllowed(f *testing.F) {
	f.Add("secret/*", "secret/db", int(CapRead))
	f.Add("**", "any/path", int(CapDeny))
	f.Add("", "", 0)

	f.Fuzz(func(t *testing.T, rulePath, reqPath string, cap int) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Allowed panicked: %v", r)
			}
		}()
		p := Policy{
			Name:  "fuzz",
			Rules: []Rule{{Path: rulePath, Capabilities: Capability(cap)}},
		}
		_ = Allowed([]Policy{p}, reqPath, Capability(cap))
	})
}
