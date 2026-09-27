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
	"math"
	"testing"

	"github.com/google/ax/internal/store"
	"github.com/google/ax/pkg/apis/v1alpha1"
)

// A client-controlled int64 limit combined with a nonzero offset overflowed
// offset+limit into a negative slice bound and panicked the store (and with it
// any RPC handler calling it).
func TestListTasksHugeLimitOffsetNoPanic(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	for _, n := range []string{"a", "b", "c"} {
		if err := s.SaveTask(ctx, &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: n}}); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := s.ListTasks(ctx, "", math.MaxInt64, 1)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks (offset 1 of 3), got %d", len(tasks))
	}
}

// Limits above the cap are clamped, not rejected; the page stays correct.
func TestListTasksLimitCappedToMaxPage(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	for _, n := range []string{"a", "b"} {
		if err := s.SaveTask(ctx, &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: n}}); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := s.ListTasks(ctx, "", math.MaxInt64, 0)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestClampListLimit(t *testing.T) {
	if got := store.ClampListLimit(50); got != 50 {
		t.Errorf("ClampListLimit(50) = %d, want 50", got)
	}
	if got := store.ClampListLimit(math.MaxInt64); got != store.MaxListPageSize {
		t.Errorf("ClampListLimit(MaxInt64) = %d, want %d", got, store.MaxListPageSize)
	}
	// The floor is untouched: each store keeps its own non-positive handling.
	if got := store.ClampListLimit(0); got != 0 {
		t.Errorf("ClampListLimit(0) = %d, want 0", got)
	}
	if got := store.ClampListLimit(-5); got != -5 {
		t.Errorf("ClampListLimit(-5) = %d, want -5", got)
	}
}
