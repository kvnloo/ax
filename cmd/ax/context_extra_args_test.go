package main

import (
	"strings"
	"testing"
)

// `ax ctx frobnicate` used to print the context and silently drop the
// extra arg. Like every other command, it must usage-error instead.
func TestRunContextRejectsExtraArgs(t *testing.T) {
	err := runContext("", []string{"frobnicate"})
	if err == nil {
		t.Fatal("runContext accepted an extra positional arg, want a usage error")
	}
	if !strings.Contains(err.Error(), "unexpected extra argument") || !strings.Contains(err.Error(), "frobnicate") {
		t.Fatalf("runContext error %q does not name the extra argument", err.Error())
	}
}

// The bare `ax ctx` path is unchanged: no args, no error from arg parsing.
func TestRunContextNoArgsParses(t *testing.T) {
	t.Setenv("KUBECONTEXT", "env-ctx")
	if err := runContext("", nil); err != nil {
		t.Fatalf("runContext with no args failed: %v", err)
	}
}
