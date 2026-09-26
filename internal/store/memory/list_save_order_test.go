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
	"time"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestListTasksResaveMovesToFront: the Redis store re-adds the task to its
// sorted-set index on every SaveTask with score = save time, so a full save
// of an existing task (e.g. `ax apply` on an existing manifest, server
// UpdateTask) bumps it to the front of the newest-first listing. The memory
// store must match that ordering — sorting by creation timestamp only
// matches on first save.
func TestListTasksResaveMovesToFront(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	base := time.Now().Add(-time.Hour)
	mk := func(name string, createdAgo time.Duration) *v1alpha1.Task {
		return &v1alpha1.Task{
			Metadata: &v1alpha1.ObjectMeta{
				Name:              name,
				Atespace:          "default",
				CreationTimestamp: timestamppb.New(base.Add(createdAgo)),
			},
		}
	}

	if err := s.SaveTask(ctx, mk("a", 0)); err != nil {
		t.Fatalf("SaveTask(a): %v", err)
	}
	if err := s.SaveTask(ctx, mk("b", time.Minute)); err != nil {
		t.Fatalf("SaveTask(b): %v", err)
	}

	// Update task "a": creation timestamp stays put (server UpdateTask
	// carries it over), but this is the newest save.
	upd := mk("a", 0)
	upd.Spec = &v1alpha1.TaskSpec{Image: "busybox:1.36"}
	if err := s.SaveTask(ctx, upd); err != nil {
		t.Fatalf("SaveTask(a) update: %v", err)
	}

	names := listNames(t, s, 0, 0)
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("expected [a b] after re-saving a (newest-save-first), got %v", names)
	}
}

// TestListTasksResaveOrderStableAcrossCalls: the save-order listing must be
// deterministic across repeated calls, including updates interleaved with
// reads.
func TestListTasksResaveOrderStableAcrossCalls(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	mk := func(name string) *v1alpha1.Task {
		return &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: name, Atespace: "default"}}
	}
	for _, n := range []string{"a", "b", "c", "d"} {
		if err := s.SaveTask(ctx, mk(n)); err != nil {
			t.Fatalf("SaveTask(%s): %v", n, err)
		}
	}
	if err := s.SaveTask(ctx, mk("b")); err != nil { // bump b
		t.Fatalf("SaveTask(b) update: %v", err)
	}

	first := listNames(t, s, 0, 0)
	want := []string{"b", "d", "c", "a"}
	for i := range want {
		if first[i] != want[i] {
			t.Fatalf("expected %v after re-saving b, got %v", want, first)
		}
	}
	second := listNames(t, s, 0, 0)
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("ListTasks order unstable across calls: %v vs %v", first, second)
		}
	}
}
