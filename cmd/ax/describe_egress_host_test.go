package main

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// HostRule.Port is schema-accepted but never enforced: ApplyEgressPolicy
// ignores it and GetPort() has no callers. Rendering "host:port" would
// assert behavior the server doesn't implement, so the label is the host
// alone — matching the get EGRESS-HOSTS column.
func TestEgressHostLabelDropsUnenforcedPort(t *testing.T) {
	if got := egressHostLabel(&v1alpha1.HostRule{Host: "example.com", Port: 443}); got != "example.com" {
		t.Fatalf("label = %q, want example.com (port is not enforced)", got)
	}
}

func TestEgressHostLabelNoPort(t *testing.T) {
	if got := egressHostLabel(&v1alpha1.HostRule{Host: "example.com"}); got != "example.com" {
		t.Fatalf("label = %q, want example.com", got)
	}
}

func TestEgressHostLabelNil(t *testing.T) {
	if got := egressHostLabel(nil); got != "" {
		t.Fatalf("label = %q, want empty", got)
	}
}
