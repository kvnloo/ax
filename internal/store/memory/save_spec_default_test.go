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

// A Task manifest without a spec: key (e.g. `ax apply -f` of a metadata-only
// manifest) decodes to a nil Spec. SaveTask must default it: the controller
// worker dereferences task.Spec when resolving the gateway, so a nil Spec
// stored by the server's UpdateTask upsert panicked the worker on the next
// reconcile event.
func TestSaveTaskNilSpecDefaulted(t *testing.T) {
	ctx := context.Background()
	s := NewStore()

	task := &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: "nospec"}}
	if task.Spec != nil {
		t.Fatal("precondition: test task should have nil Spec")
	}
	if err := s.SaveTask(ctx, task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}

	got, err := s.GetTask(ctx, "default", "nospec")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Spec == nil {
		t.Fatal("GetTask returned nil Spec for a task saved with nil Spec")
	}
}
