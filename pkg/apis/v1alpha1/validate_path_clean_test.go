package v1alpha1_test

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

func TestValidateTaskDuplicatePathUncleanedSpellings(t *testing.T) {
	task := &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "t", Atespace: "default"},
		Spec: &v1alpha1.TaskSpec{
			Workspaces: []*v1alpha1.WorkspaceRef{
				{Name: "one", Path: "/a/./b"},
				{Name: "two", Path: "/a/b"},
			},
		},
	}
	if err := v1alpha1.ValidateTask(task); err == nil {
		t.Fatalf("expected duplicate-path error for /a/./b vs /a/b, got nil")
	}
}
