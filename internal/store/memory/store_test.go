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

package memory

import (
	"context"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// TestUpdateTaskStatusDoesNotAliasCaller verifies the store's contract that
// stored resources are never shared with callers: mutating the status object
// after UpdateTaskStatus returns must not change what GetTask returns.
func TestUpdateTaskStatusDoesNotAliasCaller(t *testing.T) {
	ctx := context.Background()
	s := NewStore()

	task := &v1alpha1.Task{
		ApiVersion: v1alpha1.APIVersion,
		Kind:       v1alpha1.KindTask,
		Metadata:   &v1alpha1.ObjectMeta{Name: "alias-task", Atespace: "default"},
		Spec:       &v1alpha1.TaskSpec{},
	}
	if err := s.SaveTask(ctx, task); err != nil {
		t.Fatalf("SaveTask failed: %v", err)
	}

	status := &v1alpha1.TaskStatus{
		Phase: "Running",
		Conditions: []*v1alpha1.Condition{
			{Type: "GatewayReady", Status: "True", Reason: "PoliciesApplied"},
		},
	}
	if err := s.UpdateTaskStatus(ctx, "default", "alias-task", status); err != nil {
		t.Fatalf("UpdateTaskStatus failed: %v", err)
	}

	// Mutate the caller's object after the store accepted it.
	status.Phase = "Corrupted"
	status.Conditions[0].Status = "False"

	got, err := s.GetTask(ctx, "default", "alias-task")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got.Status.Phase != "Running" {
		t.Errorf("stored phase changed through the caller's object: got %q, want %q", got.Status.Phase, "Running")
	}
	if got.Status.Conditions[0].Status != "True" {
		t.Errorf("stored condition changed through the caller's object: got %q, want %q", got.Status.Conditions[0].Status, "True")
	}
}
