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

	"github.com/google/ax/internal/store"
	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/pkg/apis/v1alpha1"
)

// Deleting a gateway, workspace, or model that does not exist reports
// ErrNotFound so the server can answer NotFound instead of silent success.
func TestDelete_MissingReturnsNotFound(t *testing.T) {
	s := memory.NewStore()
	ctx := context.Background()

	if err := s.DeleteGateway(ctx, "default", "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("DeleteGateway missing: got %v, want ErrNotFound", err)
	}
	if err := s.DeleteWorkspace(ctx, "default", "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("DeleteWorkspace missing: got %v, want ErrNotFound", err)
	}
	if err := s.DeleteModel(ctx, "default", "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("DeleteModel missing: got %v, want ErrNotFound", err)
	}
}

// Control: deleting a saved resource succeeds and a second delete is NotFound.
func TestDelete_ExistingThenMissing(t *testing.T) {
	s := memory.NewStore()
	ctx := context.Background()

	if err := s.SaveGateway(ctx, &v1alpha1.Gateway{Metadata: &v1alpha1.ObjectMeta{Name: "gw1"}}); err != nil {
		t.Fatalf("SaveGateway failed: %v", err)
	}
	if err := s.DeleteGateway(ctx, "default", "gw1"); err != nil {
		t.Errorf("DeleteGateway existing: got %v, want nil", err)
	}
	if err := s.DeleteGateway(ctx, "default", "gw1"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("DeleteGateway after delete: got %v, want ErrNotFound", err)
	}
}
