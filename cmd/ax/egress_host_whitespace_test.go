package main

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// TestEgressHostsLabel: whitespace-only allowlist hosts are dropped, so the
// EGRESS-HOSTS column reads "<none>" (via the caller's empty check) instead
// of rendering a blank " " entry — the same whitespace-as-empty rule the
// task actor/workerIP columns follow. Red on base: the old inline loop
// joined the raw h.Host, so " " rendered instead of "".
func TestEgressHostsLabel(t *testing.T) {
	hosts := func(ss ...string) []*v1alpha1.HostRule {
		var out []*v1alpha1.HostRule
		for _, s := range ss {
			out = append(out, &v1alpha1.HostRule{Host: s})
		}
		return out
	}
	for _, tc := range []struct {
		name  string
		hosts []*v1alpha1.HostRule
		want  string
	}{
		{"nil", nil, ""},
		{"empty", hosts(), ""},
		{"whitespace only", hosts("   "), ""},
		{"mixed", hosts("  ", "example.com", "\t"), "example.com"},
		{"padded trimmed", hosts("  example.com  "), "example.com"},
		{"joined", hosts("a.com", "b.com"), "a.com,b.com"},
		{"nil entry skipped", append(hosts("a.com"), nil), "a.com"},
	} {
		if got := egressHostsLabel(tc.hosts); got != tc.want {
			t.Errorf("%s: egressHostsLabel = %q, want %q", tc.name, got, tc.want)
		}
	}
}
