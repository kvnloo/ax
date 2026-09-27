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

// SaveTask guarantees a non-nil Status, so a nil update must not reintroduce
// a nil: the next consumer that writes `task.Status.Phase` (the worker's
// reconcile-error path) would panic and kill the whole controller loop.
func TestUpdateTaskStatusNilDefaults(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	if err := s.SaveTask(ctx, &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "t", Atespace: "default"},
		Spec:     &v1alpha1.TaskSpec{},
	}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	if err := s.UpdateTaskStatus(ctx, "default", "t", nil); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	got, err := s.GetTask(ctx, "default", "t")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status == nil {
		t.Fatal("expected Status to be defaulted, got nil")
	}
}
