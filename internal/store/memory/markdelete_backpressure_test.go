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

package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/pkg/apis/v1alpha1"
)

func saveTaskForDelete(t *testing.T, st *memory.MemoryStore, ctx context.Context, name string) {
	t.Helper()
	task := &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: name, Atespace: "default"}}
	if err := st.SaveTask(ctx, task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
}

// On a full event buffer with a canceled context, MarkTaskDeleting must
// report the cancellation instead of silently dropping the delete event: a
// dropped delete event strands the task in Terminating forever (the worker
// never tears it down), so `ax delete` hangs until its context expires.
func TestMarkTaskDeletingFullBufferCanceledCtx(t *testing.T) {
	st := memory.NewStore()
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		saveTaskForDelete(t, st, ctx, "t")
	}
	saveTaskForDelete(t, st, ctx, "victim")

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := st.MarkTaskDeleting(canceled, "default", "victim"); !errors.Is(err, context.Canceled) {
		t.Fatalf("MarkTaskDeleting on full buffer with canceled ctx = %v, want context.Canceled (delete event silently dropped)", err)
	}
}

// Green path: the task is marked Terminating and the delete event is
// published and consumable through a subscription.
func TestMarkTaskDeletingPublishesDeleteEvent(t *testing.T) {
	st := memory.NewStore()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	saveTaskForDelete(t, st, ctx, "victim")
	if err := st.MarkTaskDeleting(ctx, "default", "victim"); err != nil {
		t.Fatalf("MarkTaskDeleting: %v", err)
	}

	got, err := st.GetTask(ctx, "default", "victim")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if phase := got.GetStatus().GetPhase(); phase != v1alpha1.PhaseTerminating {
		t.Fatalf("phase = %q, want %q", phase, v1alpha1.PhaseTerminating)
	}

	sub, err := st.Subscribe(ctx, "g", "c")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	foundDelete := false
	for i := 0; i < 1002; i++ {
		ev, err := sub.Next(ctx)
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if ev.Action == "delete" && ev.Name == "victim" {
			foundDelete = true
			break
		}
	}
	if !foundDelete {
		t.Fatal("delete event for victim not published")
	}
}
