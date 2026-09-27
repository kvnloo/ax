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

package server_test

import (
	"context"
	"testing"

	"github.com/google/ax/internal/server"
	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Deleting a resource that does not exist is NotFound, matching the Get
// handlers and DeleteTask. It must not surface as an Internal error.
func TestDelete_MissingResourceReturnsNotFound(t *testing.T) {
	srv := server.NewServer(memory.NewStore())
	ctx := context.Background()

	if _, err := srv.DeleteGateway(ctx, &v1alpha1.DeleteGatewayRequest{Atespace: "default", Name: "nope"}); status.Code(err) != codes.NotFound {
		t.Errorf("DeleteGateway missing: got code %v, want NotFound", status.Code(err))
	}
	if _, err := srv.DeleteWorkspace(ctx, &v1alpha1.DeleteWorkspaceRequest{Atespace: "default", Name: "nope"}); status.Code(err) != codes.NotFound {
		t.Errorf("DeleteWorkspace missing: got code %v, want NotFound", status.Code(err))
	}
	if _, err := srv.DeleteModel(ctx, &v1alpha1.DeleteModelRequest{Atespace: "default", Name: "nope"}); status.Code(err) != codes.NotFound {
		t.Errorf("DeleteModel missing: got code %v, want NotFound", status.Code(err))
	}
}

// Control: deleting an existing resource still succeeds.
func TestDelete_ExistingResourceSucceeds(t *testing.T) {
	srv := server.NewServer(memory.NewStore())
	ctx := context.Background()

	if _, err := srv.UpdateGateway(ctx, &v1alpha1.UpdateGatewayRequest{Gateway: &v1alpha1.Gateway{
		Metadata: &v1alpha1.ObjectMeta{Name: "gw1"},
		Spec:     &v1alpha1.GatewaySpec{},
	}}); err != nil {
		t.Fatalf("UpdateGateway failed: %v", err)
	}
	if _, err := srv.DeleteGateway(ctx, &v1alpha1.DeleteGatewayRequest{Atespace: "default", Name: "gw1"}); err != nil {
		t.Errorf("DeleteGateway existing: got error %v, want nil", err)
	}
	if _, err := srv.DeleteGateway(ctx, &v1alpha1.DeleteGatewayRequest{Atespace: "default", Name: "gw1"}); status.Code(err) != codes.NotFound {
		t.Errorf("DeleteGateway after delete: got code %v, want NotFound", status.Code(err))
	}
}
