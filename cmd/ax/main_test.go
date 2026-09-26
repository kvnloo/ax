package main

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// sshTargetActor must not panic when the server returns a task without
// metadata. runSSH dereferenced task.Metadata.Atespace unconditionally;
// every other Metadata access in the CLI (runGet, runDescribe) nil-checks
// first. A metadata-less task now dials with an empty atespace segment
// instead of crashing the CLI after a successful GetTask.
func TestSSHTargetActor(t *testing.T) {
	tests := []struct {
		name string
		task *v1alpha1.Task
		want string
	}{
		{
			"full task",
			&v1alpha1.Task{
				Metadata: &v1alpha1.ObjectMeta{Atespace: "prod"},
				Status:   &v1alpha1.TaskStatus{Actor: "actor-1"},
			},
			"prod/actor-1",
		},
		{
			"nil metadata does not panic",
			&v1alpha1.Task{
				Status: &v1alpha1.TaskStatus{Actor: "actor-1"},
			},
			"/actor-1",
		},
		{
			"nil status does not panic",
			&v1alpha1.Task{
				Metadata: &v1alpha1.ObjectMeta{Atespace: "prod"},
			},
			"prod/",
		},
		{
			"nil metadata and status does not panic",
			&v1alpha1.Task{},
			"/",
		},
		{
			"empty metadata",
			&v1alpha1.Task{
				Metadata: &v1alpha1.ObjectMeta{},
				Status:   &v1alpha1.TaskStatus{Actor: "actor-1"},
			},
			"/actor-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("sshTargetActor panicked: %v", r)
					}
				}()
				got = sshTargetActor(tt.task)
			}()
			if got != tt.want {
				t.Errorf("sshTargetActor() = %q, want %q", got, tt.want)
			}
		})
	}
}
