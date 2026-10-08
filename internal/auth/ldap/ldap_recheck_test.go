package ldap

import (
	"testing"
	"time"
)

func TestRecheckEvery(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", DefaultRecheckInterval, false},
		{"10m", 10 * time.Minute, false},
		{"off", 0, false},
		{"0", 0, false},
		{"5s", 0, true},
		{"soon", 0, true},
	}
	for _, c := range cases {
		cfg := Config{RecheckInterval: c.in}
		got, err := cfg.RecheckEvery()
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("%q: got %v, %v", c.in, got, err)
		}
	}
}
