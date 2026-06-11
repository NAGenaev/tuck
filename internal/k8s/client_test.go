package k8s

import (
	"testing"
)

func TestParseUsername(t *testing.T) {
	tests := []struct {
		input   string
		wantNS  string
		wantSA  string
		wantErr bool
	}{
		{"system:serviceaccount:default:myapp", "default", "myapp", false},
		{"system:serviceaccount:kube-system:coredns", "kube-system", "coredns", false},
		{"notaserviceaccount", "", "", true},
		{"", "", "", true},
	}

	for _, tc := range tests {
		ns, sa, err := ParseUsername(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseUsername(%q): expected error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseUsername(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if ns != tc.wantNS || sa != tc.wantSA {
			t.Errorf("ParseUsername(%q) = (%q, %q), want (%q, %q)",
				tc.input, ns, sa, tc.wantNS, tc.wantSA)
		}
	}
}
