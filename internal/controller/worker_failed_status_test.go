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

package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/ax/internal/store"
	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/internal/substrate"
	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// failStatusStore wraps a Store and fails every UpdateTaskStatus call.
type failStatusStore struct {
	store.Store
	updateErr error
}

func (s *failStatusStore) UpdateTaskStatus(ctx context.Context, atespace, name string, status *v1alpha1.TaskStatus) error {
	return s.updateErr
}

// TestProcessEventSurfacesFailedStatusUpdate proves that when reconciliation
// fails AND the follow-up status write fails, the returned error mentions the
// status failure instead of swallowing it. The Failed marker is the operator's
// only signal that the task will not recover on its own; losing it silently
// leaves a stale phase in the record.
func TestProcessEventSurfacesFailedStatusUpdate(t *testing.T) {
	ctx := context.Background()

	mem := memory.NewStore()
	if err := mem.SaveTask(ctx, &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "status-fail-task", Atespace: "default"},
		Spec:     &v1alpha1.TaskSpec{},
	}); err != nil {
		t.Fatalf("saving task: %v", err)
	}

	// Point the substrate client at a dead port so EnsureAtespace fails fast
	// and Reconcile returns an error at step 1.
	subClient, err := substrate.NewClient("127.0.0.1:1", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("creating substrate client: %v", err)
	}
	defer subClient.Close()
	reconciler := NewTaskReconciler(subClient, "default-template", "ax-system")

	w := NewWorker(&failStatusStore{Store: mem, updateErr: errors.New("store unavailable")}, reconciler, "g", "c")

	err = w.processEvent(ctx, store.TaskEvent{Atespace: "default", Name: "status-fail-task", Action: "reconcile"})
	if err == nil {
		t.Fatal("expected an error from the failed reconcile, got nil")
	}
	if !strings.Contains(err.Error(), "store unavailable") {
		t.Errorf("expected the returned error to mention the failed status write, got: %v", err)
	}
	if !strings.Contains(err.Error(), "reconciling task default/status-fail-task") {
		t.Errorf("expected the returned error to keep the original reconcile failure, got: %v", err)
	}
}
