// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package substrate

import (
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

func TestEgressRulesFromAllowlist_PinnedPortRejected(t *testing.T) {
	allowlist := &v1alpha1.EgressAllowlist{
		Hosts: []*v1alpha1.HostRule{{Host: "db.internal", Port: 5432}},
	}
	_, err := egressRulesFromAllowlist(allowlist)
	if err == nil {
		t.Fatal("expected an error for a port-pinned host rule, got nil")
	}
	if !strings.Contains(err.Error(), "5432") || !strings.Contains(err.Error(), "db.internal") {
		t.Errorf("error should name the host and port, got: %v", err)
	}
}

func TestEgressRulesFromAllowlist_PinnedPortOnWildcardRejected(t *testing.T) {
	allowlist := &v1alpha1.EgressAllowlist{
		Hosts: []*v1alpha1.HostRule{{Host: "*", Port: 443}},
	}
	if _, err := egressRulesFromAllowlist(allowlist); err == nil {
		t.Fatal("expected an error for a port-pinned wildcard rule, got nil")
	}
}

func TestEgressRulesFromAllowlist_HostnameAndCIDR(t *testing.T) {
	allowlist := &v1alpha1.EgressAllowlist{
		Hosts: []*v1alpha1.HostRule{
			{Host: "api.anthropic.com"},
			{Host: "10.0.0.0/8"},
		},
	}
	rules, err := egressRulesFromAllowlist(allowlist)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if got := rules[0].GetHostnames().GetPatterns(); len(got) != 1 || got[0] != "api.anthropic.com" {
		t.Errorf("unexpected hostname patterns: %v", got)
	}
	if got := rules[1].GetCidrs().GetCidrs(); len(got) != 1 || got[0] != "10.0.0.0/8" {
		t.Errorf("unexpected CIDR rules: %v", got)
	}
}

func TestEgressRulesFromAllowlist_AllowAll(t *testing.T) {
	for _, host := range []string{"*", "0.0.0.0/0"} {
		allowlist := &v1alpha1.EgressAllowlist{
			Hosts: []*v1alpha1.HostRule{{Host: host}},
		}
		rules, err := egressRulesFromAllowlist(allowlist)
		if err != nil {
			t.Fatalf("host %q: unexpected error: %v", host, err)
		}
		if len(rules) != 1 || rules[0].GetAll() == nil {
			t.Errorf("host %q: expected a single allow-all rule, got %v", host, rules)
		}
	}
}

func TestEgressRulesFromAllowlist_Empty(t *testing.T) {
	rules, err := egressRulesFromAllowlist(&v1alpha1.EgressAllowlist{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected no rules, got %v", rules)
	}
}
