package policy

import (
	"testing"
	"time"
)

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

func TestPolicy_Inheritable(t *testing.T) {
	// A policy with Inheritable=true should be usable in child namespaces.
	p := Policy{
		Name:        "shared-reader",
		Inheritable: true,
		Rules: []Rule{
			{Path: "secret/**", Capabilities: CapRead},
		},
	}
	if !p.Inheritable {
		t.Fatal("expected Inheritable=true")
	}
	if !Allowed([]Policy{p}, "secret/db", CapRead) {
		t.Fatal("expected read allowed via inherited policy")
	}
}

func TestPolicy_InheritableDefault(t *testing.T) {
	// Policies are NOT inheritable by default.
	p := Policy{Name: "local-only", Rules: []Rule{
		{Path: "secret/**", Capabilities: CapRead},
	}}
	if p.Inheritable {
		t.Fatal("expected Inheritable=false by default")
	}
}

func TestCheckRequiredParameters(t *testing.T) {
	rules := []Rule{{
		Path:               "secret/*",
		Capabilities:       CapWrite,
		RequiredParameters: []string{"password", "username"},
	}}
	// All required present.
	if missing := CheckRequiredParameters(rules, map[string]any{"password": "x", "username": "y"}); missing != "" {
		t.Errorf("expected no missing, got %q", missing)
	}
	// Missing one.
	if missing := CheckRequiredParameters(rules, map[string]any{"password": "x"}); missing != "username" {
		t.Errorf("expected missing=username, got %q", missing)
	}
}

func TestCheckDeniedParameters_withDenied(t *testing.T) {
	rules := []Rule{{
		Path:             "secret/*",
		Capabilities:     CapWrite,
		DeniedParameters: []string{"secret_key"},
	}}
	if denied := CheckDeniedParameters(rules, map[string]any{"name": "x", "secret_key": "y"}); denied != "secret_key" {
		t.Errorf("expected denied=secret_key, got %q", denied)
	}
	if denied := CheckDeniedParameters(rules, map[string]any{"name": "x"}); denied != "" {
		t.Errorf("expected no denied, got %q", denied)
	}
}

func TestCheckDeniedParameters_allowedOverrides(t *testing.T) {
	rules := []Rule{{
		Path:              "secret/*",
		Capabilities:      CapWrite,
		AllowedParameters: []string{"a", "b"},
	}}
	// "c" not in allowed list → denied.
	if denied := CheckDeniedParameters(rules, map[string]any{"a": 1, "c": 2}); denied == "" {
		t.Error("expected c to be denied (not in allowed list)")
	}
	// Only allowed params → ok.
	if denied := CheckDeniedParameters(rules, map[string]any{"a": 1, "b": 2}); denied != "" {
		t.Errorf("expected no denied, got %q", denied)
	}
}

func TestFilterParameters(t *testing.T) {
	rules := []Rule{{
		Path:              "secret/*",
		AllowedParameters: []string{"a", "b"},
	}}
	filtered := FilterParameters(rules, map[string]any{"a": 1, "b": 2, "c": 3})
	if _, ok := filtered["c"]; ok {
		t.Error("filtered map must not contain c")
	}
	if filtered["a"] != 1 || filtered["b"] != 2 {
		t.Errorf("filtered map = %v", filtered)
	}
}

func TestFilterParameters_noAllowed(t *testing.T) {
	rules := []Rule{{Path: "secret/*", Capabilities: CapRead}}
	in := map[string]any{"a": 1, "b": 2}
	out := FilterParameters(rules, in)
	if len(out) != 2 {
		t.Errorf("without AllowedParameters, FilterParameters must return unchanged map")
	}
}

func TestRequiredMFAMethods(t *testing.T) {
	rules := []Rule{
		{MFAMethods: []string{"totp", "duo"}},
		{MFAMethods: []string{"totp"}},
	}
	methods := RequiredMFAMethods(rules)
	if len(methods) != 2 {
		t.Errorf("expected 2 unique methods, got %v", methods)
	}
}

func TestWrappingBounds(t *testing.T) {
	rules := []Rule{
		{MinWrappingTTL: 5 * time.Minute, MaxWrappingTTL: 2 * time.Hour},
		{MinWrappingTTL: 10 * time.Minute, MaxWrappingTTL: time.Hour},
	}
	min, max := WrappingBounds(rules)
	if min != 10*time.Minute {
		t.Errorf("min = %v, want 10m (tightest lower bound)", min)
	}
	if max != time.Hour {
		t.Errorf("max = %v, want 1h (tightest upper bound)", max)
	}
}

func TestWrappingBounds_none(t *testing.T) {
	rules := []Rule{{Path: "secret/*", Capabilities: CapRead}}
	min, max := WrappingBounds(rules)
	if min != 0 || max != 0 {
		t.Errorf("no wrapping bounds: got min=%v max=%v", min, max)
	}
}

func TestMatchingRules(t *testing.T) {
	p := Policy{Name: "test", Rules: []Rule{
		{Path: "secret/db/*", Capabilities: CapRead},
		{Path: "auth/**", Capabilities: CapWrite},
	}}
	rules := MatchingRules([]Policy{p}, "secret/db/password")
	if len(rules) != 1 || rules[0].Path != "secret/db/*" {
		t.Errorf("MatchingRules = %v", rules)
	}
	rules = MatchingRules([]Policy{p}, "other/path")
	if len(rules) != 0 {
		t.Errorf("expected no matching rules, got %v", rules)
	}
}

func TestPolicy_MarshalRoundTrip(t *testing.T) {
	import_p := Policy{
		Name:        "ns-policy",
		Inheritable: true,
		Rules: []Rule{
			{Path: "secret/*", Capabilities: CapRead | CapList},
		},
	}
	data, err := import_p.marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "ns-policy" || !got.Inheritable || len(got.Rules) != 1 {
		t.Fatalf("unexpected unmarshalled policy: %+v", got)
	}
}
