package main

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// A whitespace-only actor must fail like the empty case: the old code built
// an "atespace/ " dial target and failed deep inside the router with an
// opaque error instead of naming the real problem.
func TestSSHTargetActorWhitespaceOnlyActorErrors(t *testing.T) {
	for _, actor := range []string{" ", "  ", "\t"} {
		task := &v1alpha1.Task{
			Metadata: &v1alpha1.ObjectMeta{Name: "t", Atespace: "default"},
			Status:   &v1alpha1.TaskStatus{Actor: actor},
		}
		if target, err := sshTargetActor(task); err == nil {
			t.Errorf("sshTargetActor(actor=%q) = %q, want error", actor, target)
		}
	}
}

// A padded-but-real actor must dial the trimmed name, not the padded one.
func TestSSHTargetActorTrimsPaddedActor(t *testing.T) {
	task := &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "t", Atespace: "prod"},
		Status:   &v1alpha1.TaskStatus{Actor: " actor-1 "},
	}
	target, err := sshTargetActor(task)
	if err != nil {
		t.Fatalf("sshTargetActor: %v", err)
	}
	if target != "prod/actor-1" {
		t.Errorf("sshTargetActor = %q, want %q", target, "prod/actor-1")
	}
}
