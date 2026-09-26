package main

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// A whitespace-only phase must read like the empty case ("Pending"), not
// render a blank PHASE column — the same hole chunk 34 fixed for "".
// Reachable: the stores default only "" to "Pending" on save.
func TestDescribeTaskPhaseWhitespaceOnly(t *testing.T) {
	task := &v1alpha1.Task{Status: &v1alpha1.TaskStatus{Phase: "   "}}
	if got := describeTaskPhase(task); got != "Pending" {
		t.Fatalf("describeTaskPhase(whitespace) = %q, want %q", got, "Pending")
	}
}

func TestDescribeTaskPhaseWhitespacePaddedPassesTrimmed(t *testing.T) {
	task := &v1alpha1.Task{Status: &v1alpha1.TaskStatus{Phase: " Running "}}
	if got := describeTaskPhase(task); got != "Running" {
		t.Fatalf("describeTaskPhase(padded) = %q, want %q", got, "Running")
	}
}

// displayPhase is the shared rule behind `ax describe task` and the
// `ax get tasks` PHASE column.
func TestDisplayPhase(t *testing.T) {
	cases := map[string]string{
		"":        "Pending",
		"   ":      "Pending",
		"\t":       "Pending",
		"Running":   "Running",
		" Running ": "Running",
		"Failed":    "Failed",
	}
	for in, want := range cases {
		if got := displayPhase(in); got != want {
			t.Fatalf("displayPhase(%q) = %q, want %q", in, got, want)
		}
	}
}
