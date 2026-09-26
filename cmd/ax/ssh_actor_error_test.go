package main

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// A task with no actor must fail with a clear error before any dial: the
// old code built an "atespace/" dial target and failed deep inside the
// router with an opaque error instead of naming the real problem.
func TestSSHTargetActorEmptyActorErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		task *v1alpha1.Task
	}{
		{"empty actor", &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: "t", Atespace: "default"}, Status: &v1alpha1.TaskStatus{}}},
		{"nil status", &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: "t", Atespace: "default"}}},
		{"nil task", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var target string
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("sshTargetActor panicked: %v", r)
					}
				}()
				target, err = sshTargetActor(tc.task)
			}()
			if err == nil {
				t.Fatalf("expected error, got target %q", target)
			}
		})
	}
}

func TestSSHTargetActorStillBuildsTarget(t *testing.T) {
	target, err := sshTargetActor(&v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "t", Atespace: "prod"},
		Status:   &v1alpha1.TaskStatus{Actor: "actor-1"},
	})
	if err != nil {
		t.Fatalf("sshTargetActor: %v", err)
	}
	if target != "prod/actor-1" {
		t.Errorf("sshTargetActor = %q, want %q", target, "prod/actor-1")
	}
}
