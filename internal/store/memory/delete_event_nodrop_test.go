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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/ax/internal/store"
	"github.com/google/ax/pkg/apis/v1alpha1"
)

// fillEvents saturates the store's event buffer so subsequent publishes must
// wait for a reader instead of succeeding immediately.
func fillEvents(s *MemoryStore) {
	n := cap(s.events) - len(s.events)
	for i := 0; i < n; i++ {
		s.events <- store.TaskEvent{ID: fmt.Sprintf("fill-%d", i)}
	}
}

func saveTerminableTask(t *testing.T, s *MemoryStore, name string) {
	t.Helper()
	if err := s.SaveTask(context.Background(), &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: name, Atespace: "default"},
		Spec:     &v1alpha1.TaskSpec{},
	}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
}

// A delete event is edge-triggered: it is the only signal that tells the
// controller to tear down the actor. When the event buffer is full it must not
// be silently dropped (leaving the task stuck in Terminating with a leaked
// actor); a cancelled caller must get its cancellation back instead of a
// false success.
func TestMarkTaskDeletingFullBufferCanceledCtx(t *testing.T) {
	s := NewStore()
	saveTerminableTask(t, s, "doomed")
	fillEvents(s)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.MarkTaskDeleting(ctx, "default", "doomed")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// The phase flip already happened before the publish attempt, so the
	// record stays visibly Terminating and a later `ax delete` retries.
	got, err := s.GetTask(context.Background(), "default", "doomed")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status.Phase != v1alpha1.PhaseTerminating {
		t.Errorf("expected phase Terminating after failed publish, got %q", got.Status.Phase)
	}
}

// When a reader is draining the buffer, the delete event must still be
// delivered (no deadlock, no drop).
func TestMarkTaskDeletingFullBufferDrains(t *testing.T) {
	s := NewStore()
	saveTerminableTask(t, s, "doomed")
	fillEvents(s)

	// Drain everything the filler put in, then keep reading for the delete.
	done := make(chan store.TaskEvent, 1)
	go func() {
		for {
			ev := <-s.events
			if ev.Action == "delete" && ev.Name == "doomed" {
				done <- ev
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.MarkTaskDeleting(ctx, "default", "doomed"); err != nil {
		t.Fatalf("MarkTaskDeleting: %v", err)
	}
	select {
	case ev := <-done:
		if ev.Atespace != "default" || ev.Name != "doomed" {
			t.Errorf("unexpected delete event %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("delete event was not delivered")
	}
}
