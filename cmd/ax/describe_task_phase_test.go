package main

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// The get list defaults an unset phase to "Pending" and both stores set
// "Pending" on creation; describe must agree instead of printing a blank
// Phase line for a status-less task.
func TestDescribeTaskPhaseNilStatus(t *testing.T) {
	if got := describeTaskPhase(&v1alpha1.Task{}); got != "Pending" {
		t.Fatalf("phase = %q, want Pending", got)
	}
}

func TestDescribeTaskPhaseNilTask(t *testing.T) {
	if got := describeTaskPhase(nil); got != "Pending" {
		t.Fatalf("phase = %q, want Pending", got)
	}
}

func TestDescribeTaskPhaseEmptyString(t *testing.T) {
	task := &v1alpha1.Task{Status: &v1alpha1.TaskStatus{Phase: ""}}
	if got := describeTaskPhase(task); got != "Pending" {
		t.Fatalf("phase = %q, want Pending", got)
	}
}

func TestDescribeTaskPhaseSetPassesThrough(t *testing.T) {
	task := &v1alpha1.Task{Status: &v1alpha1.TaskStatus{Phase: "Running"}}
	if got := describeTaskPhase(task); got != "Running" {
		t.Fatalf("phase = %q, want Running", got)
	}
}
