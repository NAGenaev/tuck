package github_test

import (
	"testing"

	"github.com/NAGenaev/tuck/internal/auth/github"
)

func TestSubjectFor(t *testing.T) {
	tests := []struct {
		repo, qualifier, value, want string
	}{
		{"myorg/myrepo", "environment", "production", "repo:myorg/myrepo:environment:production"},
		{"myorg/myrepo", "ref", "refs/heads/main", "repo:myorg/myrepo:ref:refs/heads/main"},
		{"myorg/myrepo", "", "", "repo:myorg/myrepo"},
	}
	for _, tc := range tests {
		got := github.SubjectFor(tc.repo, tc.qualifier, tc.value)
		if got != tc.want {
			t.Errorf("SubjectFor(%q,%q,%q) = %q, want %q", tc.repo, tc.qualifier, tc.value, got, tc.want)
		}
	}
}

func TestNewProvider(t *testing.T) {
	p := github.NewProvider()
	if p == nil {
		t.Fatal("NewProvider returned nil")
	}
}

func TestRoleConstants(t *testing.T) {
	if github.GitHubIssuer == "" {
		t.Error("GitHubIssuer is empty")
	}
	if github.GitHubJWKSURI == "" {
		t.Error("GitHubJWKSURI is empty")
	}
}
