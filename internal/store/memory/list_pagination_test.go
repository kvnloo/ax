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
	"fmt"
	"testing"
	"time"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func saveTaskNames(t *testing.T, s *MemoryStore, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("task-%03d", i)
		task := &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: name, Atespace: "default"}}
		if err := s.SaveTask(ctx, task); err != nil {
			t.Fatalf("SaveTask(%s): %v", name, err)
		}
	}
}

func listNames(t *testing.T, s *MemoryStore, limit, offset int64) []string {
	t.Helper()
	tasks, err := s.ListTasks(context.Background(), "default", limit, offset)
	if err != nil {
		t.Fatalf("ListTasks(limit=%d, offset=%d): %v", limit, offset, err)
	}
	names := make([]string, len(tasks))
	for i, task := range tasks {
		names[i] = task.Metadata.Name
	}
	return names
}

// TestListTasksStableOrder: paging over ListTasks must be deterministic.
// The memory store iterated its task map in random order, so two identical
// ListTasks calls could return different sequences — and a client paging
// with limit/offset (e.g. `ax get tasks`, page size 100) saw duplicate rows
// and missed rows once the fleet exceeded one page.
func TestListTasksStableOrder(t *testing.T) {
	s := NewStore()
	saveTaskNames(t, s, 150)

	first := listNames(t, s, 100, 0)
	second := listNames(t, s, 100, 0)
	if len(first) != len(second) {
		t.Fatalf("ListTasks page length changed between calls: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("ListTasks order unstable across calls: position %d was %q then %q", i, first[i], second[i])
		}
	}

	// Paging to the end must cover the whole set exactly once.
	seen := map[string]int{}
	for offset := int64(0); ; offset += 100 {
		names := listNames(t, s, 100, offset)
		if len(names) == 0 {
			break
		}
		for _, n := range names {
			seen[n]++
		}
		if len(names) < 100 {
			break
		}
	}
	if len(seen) != 150 {
		t.Fatalf("paged ListTasks union = %d unique tasks, want 150 (rows duplicated or skipped)", len(seen))
	}
	for n, c := range seen {
		if c != 1 {
			t.Fatalf("paged ListTasks returned %q %d times, want exactly once", n, c)
		}
	}
}

// TestListTasksNewestFirst: the memory store matches the Redis store's
// documented order (newest first) so the two backends list identically.
func TestListTasksNewestFirst(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	old := &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{
		Name:              "old",
		Atespace:          "default",
		CreationTimestamp: timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	}}
	newer := &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{
		Name:              "new",
		Atespace:          "default",
		CreationTimestamp: timestamppb.New(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)),
	}}
	if err := s.SaveTask(ctx, old); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveTask(ctx, newer); err != nil {
		t.Fatal(err)
	}
	got := listNames(t, s, 50, 0)
	if len(got) != 2 || got[0] != "new" || got[1] != "old" {
		t.Fatalf("ListTasks = %v, want [new old] (newest first)", got)
	}
}
