package policy

import "testing"

func TestAllowed_basicAllow(t *testing.T) {
	p := Policy{
		Name: "test",
		Rules: []Rule{
			{Path: "secret/*", Capabilities: CapRead | CapWrite},
		},
	}
	if !Allowed([]Policy{p}, "secret/db", CapRead) {
		t.Fatal("expected read allowed on secret/db")
	}
	if Allowed([]Policy{p}, "secret/db", CapDelete) {
		t.Fatal("expected delete denied on secret/db")
	}
}

func TestAllowed_denyOverridesAllow(t *testing.T) {
	allow := Policy{
		Name:  "allow",
		Rules: []Rule{{Path: "secret/**", Capabilities: CapRead | CapWrite | CapDelete | CapList}},
	}
	deny := Policy{
		Name:  "deny",
		Rules: []Rule{{Path: "secret/prod/*", Capabilities: CapDeny}},
	}
	// general path: allowed
	if !Allowed([]Policy{allow, deny}, "secret/dev/key", CapRead) {
		t.Fatal("expected read allowed on dev path")
	}
	// denied path: even though allow policy covers it, deny wins
	if Allowed([]Policy{allow, deny}, "secret/prod/key", CapRead) {
		t.Fatal("expected read denied on prod path due to deny rule")
	}
}

func TestAllowed_denyOnlyPolicy(t *testing.T) {
	p := Policy{
		Name:  "block",
		Rules: []Rule{{Path: "**", Capabilities: CapDeny}},
	}
	if Allowed([]Policy{p}, "any/path", CapRead) {
		t.Fatal("expected all access denied")
	}
}

func TestAllowed_denyDoesNotMatchOtherPaths(t *testing.T) {
	p := Policy{
		Name: "mixed",
		Rules: []Rule{
			{Path: "secret/prod/*", Capabilities: CapDeny},
			{Path: "secret/**", Capabilities: CapRead},
		},
	}
	if !Allowed([]Policy{p}, "secret/dev/key", CapRead) {
		t.Fatal("expected read allowed on dev path")
	}
	if Allowed([]Policy{p}, "secret/prod/key", CapRead) {
		t.Fatal("expected read denied on prod path")
	}
}

func TestAllowed_rootPolicyUnaffectedByDenyInOtherPolicy(t *testing.T) {
	deny := Policy{
		Name:  "deny-secret",
		Rules: []Rule{{Path: "secret/**", Capabilities: CapDeny}},
	}
	// root + deny policy: deny wins (operator explicitly attached a deny policy)
	if Allowed([]Policy{RootPolicy, deny}, "secret/foo", CapRead) {
		t.Fatal("deny rule should override even root policy allow")
	}
	// root policy alone: always allowed
	if !Allowed([]Policy{RootPolicy}, "any/path", CapRead) {
		t.Fatal("root policy should allow all reads")
	}
}

func TestAllowed_noMatchReturnsFalse(t *testing.T) {
	p := Policy{
		Name:  "narrow",
		Rules: []Rule{{Path: "secret/a", Capabilities: CapRead}},
	}
	if Allowed([]Policy{p}, "secret/b", CapRead) {
		t.Fatal("expected no match to deny")
	}
}

func TestAllowed_noPoliciesReturnsFalse(t *testing.T) {
	if Allowed(nil, "secret/any", CapRead) {
		t.Fatal("expected false with no policies")
	}
}
