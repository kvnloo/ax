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
	"testing"

	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/pkg/apis/v1alpha1"
)

// UpdateTaskStatus must not keep a reference to the caller's status object:
// mutating the caller's copy afterwards must leave the stored task alone.
func TestUpdateTaskStatusClonesCallerStatus(t *testing.T) {
	st := memory.NewStore()
	ctx := context.Background()
	if err := st.SaveTask(ctx, &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: "t", Atespace: "default"}}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}

	status := &v1alpha1.TaskStatus{Phase: "Running", Actor: "t"}
	if err := st.UpdateTaskStatus(ctx, "default", "t", status); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}

	// Mutate the caller's object after the update; the store must be unaffected.
	status.Phase = "Failed"
	status.Actor = "rewritten"

	got, err := st.GetTask(ctx, "default", "t")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.GetStatus().GetPhase() != "Running" || got.GetStatus().GetActor() != "t" {
		t.Fatalf("stored status rewritten by caller mutation: phase=%q actor=%q",
			got.GetStatus().GetPhase(), got.GetStatus().GetActor())
	}
}

// A nil status update must not reintroduce a nil Status into the store.
func TestUpdateTaskStatusNilDefaultsNonNil(t *testing.T) {
	st := memory.NewStore()
	ctx := context.Background()
	if err := st.SaveTask(ctx, &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: "t", Atespace: "default"}}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	if err := st.UpdateTaskStatus(ctx, "default", "t", nil); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	got, err := st.GetTask(ctx, "default", "t")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.GetStatus() == nil {
		t.Fatal("UpdateTaskStatus(nil) stored a nil Status")
	}
}
