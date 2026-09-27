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
)

// MarkTaskDeleting publishes a delete event whose Atespace must name the
// atespace the task actually lives in. SaveTask folds an empty atespace to
// "default", and the Redis backend normalizes before publishing, so a memory
// event carrying the raw "" diverges from both.
func TestMarkTaskDeletingEmptyAtespaceEvent(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	if err := s.SaveTask(ctx, &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "t1"},
	}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}

	sub, err := s.Subscribe(ctx, "g", "c")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	if err := s.MarkTaskDeleting(ctx, "", "t1"); err != nil {
		t.Fatalf("MarkTaskDeleting: %v", err)
	}

	got, err := s.GetTask(ctx, "", "t1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Metadata.Atespace != "default" {
		t.Fatalf("stored atespace = %q, want default", got.Metadata.Atespace)
	}
	if got.Status == nil || got.Status.Phase != v1alpha1.PhaseTerminating {
		t.Fatalf("phase not Terminating: %+v", got.Status)
	}

	nctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// SaveTask's "reconcile" event precedes the delete event; drain it.
	if ev, err := sub.Next(nctx); err != nil || ev.Action != "reconcile" {
		t.Fatalf("first event = %+v, err = %v, want reconcile", ev, err)
	}
	ev, err := sub.Next(nctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if ev.Action != "delete" || ev.Name != "t1" {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if ev.Atespace != "default" {
		t.Fatalf("event atespace = %q, want default (task lives in default)", ev.Atespace)
	}
}
