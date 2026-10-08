package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	authlda "github.com/NAGenaev/tuck/internal/auth/ldap"
	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
	"github.com/NAGenaev/tuck/internal/namespace"
	"github.com/NAGenaev/tuck/internal/policy"
)

// Team onboarding creates everything a new team needs in one step: the
// namespace, three policies inside it, one LDAP role per access level that
// maps the team's domain groups onto those policies, and optionally
// Kubernetes ServiceAccount roles for workloads that log in by themselves.
//
// Domain groups are only ever bound here, by a platform administrator. The
// policies handed to the team do not let it create auth roles, so a team lead
// cannot widen access to its namespace to other groups.

// Access levels a team can grant, in ascending order of privilege.
const (
	AccessReader    = "reader"
	AccessDeveloper = "developer"
	AccessAdmin     = "admin"
)

var accessLevels = []string{AccessAdmin, AccessDeveloper, AccessReader}

var (
	// ErrOnboardInvalid wraps every problem with the submitted team spec.
	ErrOnboardInvalid = errors.New("invalid team spec")
	// ErrOnboardConflict means something the team needs already exists.
	ErrOnboardConflict = errors.New("team onboarding conflict")
)

// TeamK8sBinding lets one ServiceAccount of one cluster log in to the team's
// namespace.
type TeamK8sBinding struct {
	// Cluster is the trusted cluster name; empty = the default cluster.
	Cluster        string `json:"cluster,omitempty"`
	Namespace      string `json:"namespace"`
	ServiceAccount string `json:"service_account"`
	// Access is "reader" (default) or "developer".
	Access string `json:"access,omitempty"`
}

// TeamSpec describes a team to onboard.
type TeamSpec struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Owner       string            `json:"owner,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	// Groups maps an access level (admin, developer, reader) to the LDAP
	// groups that receive it.
	Groups map[string][]string `json:"groups"`
	// TTL of tokens issued through the created roles; zero = server default.
	TTL time.Duration `json:"-"`
	// K8s lists ServiceAccounts that may log in directly (injector, CSI).
	// The operator needs none: it uses its own identity and the namespace
	// annotation printed in K8sHints.
	K8s []TeamK8sBinding `json:"k8s,omitempty"`
}

// PlanItem is one object the onboarding creates (or would create).
type PlanItem struct {
	Kind   string `json:"kind"`   // namespace | policy | ldap_role | k8s_role
	Name   string `json:"name"`   // identifying name
	Action string `json:"action"` // create | conflict
	Detail string `json:"detail,omitempty"`
}

// K8sHint is the manual step left on the cluster side.
type K8sHint struct {
	Cluster    string `json:"cluster,omitempty"`
	Namespace  string `json:"namespace"`
	Annotation string `json:"annotation"`
}

// OnboardResult is what a dry run would do, or what was done.
type OnboardResult struct {
	Namespace string     `json:"namespace"`
	DryRun    bool       `json:"dry_run"`
	Items     []PlanItem `json:"items"`
	K8sHints  []K8sHint  `json:"k8s_hints,omitempty"`
}

// Conflicts returns the items that block the onboarding.
func (r *OnboardResult) Conflicts() []PlanItem {
	var out []PlanItem
	for _, it := range r.Items {
		if it.Action == "conflict" {
			out = append(out, it)
		}
	}
	return out
}

func teamPolicyName(access string) string { return "team-" + access }

// TeamLDAPRoleName is the LDAP role name onboarding uses for one access level.
// The role table is global (shared by all namespaces), so the name carries the
// namespace; "/" is not valid in a role name.
func TeamLDAPRoleName(ns, access string) string {
	return strings.ReplaceAll(ns, "/", "--") + "-" + access
}

// teamPolicies are the policies created inside the team's namespace. None of
// them touch auth roles, tokens or other namespaces.
func teamPolicies() map[string]*policy.Policy {
	const (
		r = policy.CapRead
		w = policy.CapWrite
		d = policy.CapDelete
		l = policy.CapList
	)
	mk := func(access string, secrets policy.Capability, extra ...policy.Rule) *policy.Policy {
		rules := append([]policy.Rule{{Path: "secret/**", Capabilities: secrets}}, extra...)
		return &policy.Policy{Name: teamPolicyName(access), Rules: rules}
	}
	return map[string]*policy.Policy{
		AccessAdmin: mk(AccessAdmin, r|w|d|l,
			policy.Rule{Path: "auth/policy", Capabilities: l},
			policy.Rule{Path: "auth/policy/**", Capabilities: r | l}),
		AccessDeveloper: mk(AccessDeveloper, r|w|l),
		AccessReader:    mk(AccessReader, r|l),
	}
}

// normalize validates the spec and returns a cleaned copy.
func (c *Core) normalizeTeamSpec(spec TeamSpec) (TeamSpec, error) {
	invalid := func(format string, a ...any) (TeamSpec, error) {
		return spec, fmt.Errorf("%w: %s", ErrOnboardInvalid, fmt.Sprintf(format, a...))
	}
	if err := namespace.ValidateName(spec.Name); err != nil {
		return invalid("%v", err)
	}
	if err := (namespace.Metadata{Description: spec.Description, Owner: spec.Owner, Labels: spec.Labels}).Validate(); err != nil {
		return invalid("%v", err)
	}
	groups := map[string][]string{}
	total := 0
	for access, gs := range spec.Groups {
		if access != AccessAdmin && access != AccessDeveloper && access != AccessReader {
			return invalid("unknown access level %q (valid: admin, developer, reader)", access)
		}
		seen := map[string]bool{}
		for _, g := range gs {
			g = strings.TrimSpace(g)
			if g == "" || seen[g] {
				continue
			}
			seen[g] = true
			groups[access] = append(groups[access], g)
			total++
		}
	}
	if total == 0 {
		return invalid("at least one domain group is required")
	}
	spec.Groups = groups
	seen := map[string]bool{}
	for i, b := range spec.K8s {
		if b.Namespace == "" || b.ServiceAccount == "" {
			return invalid("k8s binding %d: namespace and service_account are required", i+1)
		}
		switch b.Access {
		case "":
			spec.K8s[i].Access = AccessReader
		case AccessReader, AccessDeveloper:
		default:
			return invalid("k8s binding %d: access must be reader or developer", i+1)
		}
		if b.Cluster != "" && !k8sauth.ValidClusterName(b.Cluster) {
			return invalid("k8s binding %d: invalid cluster name %q", i+1, b.Cluster)
		}
		key := b.Cluster + "\x00" + b.Namespace + "\x00" + b.ServiceAccount
		if seen[key] {
			return invalid("k8s binding %d duplicates an earlier one", i+1)
		}
		seen[key] = true
	}
	return spec, nil
}

func k8sRoleLabel(b TeamK8sBinding) string {
	if b.Cluster == "" {
		return b.Namespace + "/" + b.ServiceAccount
	}
	return b.Cluster + "/" + b.Namespace + "/" + b.ServiceAccount
}

// PlanTeam validates a team spec and reports what OnboardTeam would create,
// marking objects that already exist as conflicts. It changes nothing.
func (c *Core) PlanTeam(ctx context.Context, spec TeamSpec) (*OnboardResult, error) {
	spec, err := c.normalizeTeamSpec(spec)
	if err != nil {
		return nil, err
	}
	res := &OnboardResult{Namespace: spec.Name, DryRun: true}

	nsItem := PlanItem{Kind: "namespace", Name: spec.Name, Action: "create"}
	if existing, err := c.namespaces.Get(ctx, spec.Name); err == nil && existing != nil {
		nsItem.Action, nsItem.Detail = "conflict", "namespace already exists"
	}
	res.Items = append(res.Items, nsItem)

	existingRoles := map[string]bool{}
	names, err := c.ListLDAPRoles(ctx)
	if err != nil {
		return nil, err
	}
	for _, n := range names {
		existingRoles[n] = true
	}

	used := map[string]bool{}
	for _, access := range accessLevels {
		if len(spec.Groups[access]) == 0 {
			continue
		}
		used[access] = true
	}
	for _, b := range spec.K8s {
		used[b.Access] = true
	}
	for _, access := range accessLevels {
		if used[access] {
			res.Items = append(res.Items, PlanItem{Kind: "policy", Name: teamPolicyName(access), Action: "create"})
		}
	}
	for _, access := range accessLevels {
		if len(spec.Groups[access]) == 0 {
			continue
		}
		it := PlanItem{
			Kind: "ldap_role", Name: TeamLDAPRoleName(spec.Name, access), Action: "create",
			Detail: strings.Join(spec.Groups[access], ", "),
		}
		if existingRoles[it.Name] {
			it.Action, it.Detail = "conflict", "LDAP role with this name already exists"
		}
		res.Items = append(res.Items, it)
	}

	hinted := map[string]bool{}
	for _, b := range spec.K8s {
		it := PlanItem{Kind: "k8s_role", Name: k8sRoleLabel(b), Action: "create", Detail: b.Access}
		if b.Cluster != "" {
			if _, err := c.k8sReviewerFor(b.Cluster); err != nil {
				return nil, fmt.Errorf("%w: cluster %q: %v", ErrOnboardInvalid, b.Cluster, err)
			}
		} else if _, err := c.k8sReviewerFor(""); err != nil {
			return nil, fmt.Errorf("%w: default kubernetes cluster: %v", ErrOnboardInvalid, err)
		}
		if _, err := c.k8sRoles.Get(ctx, b.Cluster, b.Namespace, b.ServiceAccount); err == nil {
			it.Action, it.Detail = "conflict", "role for this ServiceAccount already exists"
		} else if !errors.Is(err, k8sauth.ErrRoleNotFound) {
			return nil, err
		}
		res.Items = append(res.Items, it)

		key := b.Cluster + "\x00" + b.Namespace
		if !hinted[key] {
			hinted[key] = true
			res.K8sHints = append(res.K8sHints, K8sHint{
				Cluster: b.Cluster, Namespace: b.Namespace,
				Annotation: "tuck.io/tuck-namespace: " + spec.Name,
			})
		}
	}
	return res, nil
}

// OnboardTeam creates the namespace, policies and roles described by spec. It
// is all-or-nothing: on the first failure everything it created is removed
// again. An existing namespace or role is never overwritten (ErrOnboardConflict).
func (c *Core) OnboardTeam(ctx context.Context, spec TeamSpec) (*OnboardResult, error) {
	plan, err := c.PlanTeam(ctx, spec)
	if err != nil {
		return nil, err
	}
	plan.DryRun = false
	if conflicts := plan.Conflicts(); len(conflicts) > 0 {
		parts := make([]string, len(conflicts))
		for i, it := range conflicts {
			parts[i] = it.Kind + " " + it.Name + ": " + it.Detail
		}
		return plan, fmt.Errorf("%w: %s", ErrOnboardConflict, strings.Join(parts, "; "))
	}
	spec, _ = c.normalizeTeamSpec(spec) // already validated by PlanTeam

	var undo []func(context.Context) error
	rollback := func(cause error) error {
		// The request context may be the reason we are here; clean up regardless.
		rctx := context.WithoutCancel(ctx)
		var errs []error
		for i := len(undo) - 1; i >= 0; i-- {
			if err := undo[i](rctx); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("%w (rollback incomplete: %v)", cause, errors.Join(errs...))
		}
		return cause
	}

	if _, err := c.CreateNamespaceWithMetadata(ctx, spec.Name, namespace.Metadata{
		Description: spec.Description, Owner: spec.Owner, Labels: spec.Labels,
	}); err != nil {
		return nil, rollback(fmt.Errorf("create namespace: %w", err))
	}
	undo = append(undo, func(ctx context.Context) error { return c.DeleteNamespace(ctx, spec.Name) })

	templates := teamPolicies()
	for _, it := range plan.Items {
		if it.Kind != "policy" {
			continue
		}
		access := strings.TrimPrefix(it.Name, "team-")
		if err := c.PutPolicy(ctx, spec.Name, templates[access]); err != nil {
			return nil, rollback(fmt.Errorf("create policy %s: %w", it.Name, err))
		}
	}
	// Policies live inside the namespace, so deleting it removes them too.

	for _, access := range accessLevels {
		if len(spec.Groups[access]) == 0 {
			continue
		}
		role := &authlda.Role{
			Name:          TeamLDAPRoleName(spec.Name, access),
			Groups:        spec.Groups[access],
			Policies:      []string{teamPolicyName(access)},
			TTL:           spec.TTL,
			TuckNamespace: spec.Name,
		}
		if err := c.PutLDAPRole(ctx, role); err != nil {
			return nil, rollback(fmt.Errorf("create LDAP role %s: %w", role.Name, err))
		}
		name := role.Name
		undo = append(undo, func(ctx context.Context) error { return c.DeleteLDAPRole(ctx, name) })
	}

	for _, b := range spec.K8s {
		role := &k8sauth.K8sRole{
			Cluster: b.Cluster, Namespace: b.Namespace, ServiceAccount: b.ServiceAccount,
			Policies: []string{teamPolicyName(b.Access)}, TTL: spec.TTL, TuckNamespace: spec.Name,
		}
		if err := c.CreateK8sRole(ctx, role); err != nil {
			return nil, rollback(fmt.Errorf("create k8s role %s: %w", k8sRoleLabel(b), err))
		}
		b := b
		undo = append(undo, func(ctx context.Context) error {
			return c.DeleteK8sRole(ctx, b.Cluster, b.Namespace, b.ServiceAccount)
		})
	}
	return plan, nil
}
