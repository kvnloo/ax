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

package controller_test

import (
	"context"
	"net"
	"testing"

	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
	"github.com/google/ax/internal/controller"
	"github.com/google/ax/internal/substrate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// startDeleteMock starts the mock control server with the given template state
// and returns a TaskReconciler wired to it.
func startDeleteMock(t *testing.T, mockSrv *mockControlServer) *controller.TaskReconciler {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	t.Cleanup(func() { lis.Close() })

	grpcServer := grpc.NewServer()
	ateapipb.RegisterControlServer(grpcServer, mockSrv)
	go grpcServer.Serve(lis)
	t.Cleanup(grpcServer.Stop)

	client, err := substrate.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to create substrate client: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	return controller.NewTaskReconciler(client, "test-template", "ax-system")
}

// A template-cleanup failure must surface from ReconcileDelete. The worker keeps
// the task record in Terminating when ReconcileDelete errors, so the failure is
// visible and a later `ax delete` retries. Swallowing the error deletes the
// record and orphans the templates with no retry path.
func TestReconcileDeleteReturnsTemplateCleanupError(t *testing.T) {
	ctx := context.Background()
	mockSrv := &mockControlServer{
		actorTemplates:    map[string]bool{"my-task-tmpl-deadbeef": true},
		deleteTemplateErr: status.Error(codes.PermissionDenied, "denied"),
	}
	reconciler := startDeleteMock(t, mockSrv)

	err := reconciler.ReconcileDelete(ctx, "default", "my-task")
	if err == nil {
		t.Fatal("ReconcileDelete returned nil despite persistent template cleanup failure; the orphaned template would have no retry path")
	}
}

// The happy path still succeeds: actor deleted, template cleaned up, nil error.
func TestReconcileDeleteCleansTemplates(t *testing.T) {
	ctx := context.Background()
	mockSrv := &mockControlServer{
		actorTemplates: map[string]bool{"my-task-tmpl-deadbeef": true},
	}
	reconciler := startDeleteMock(t, mockSrv)

	if err := reconciler.ReconcileDelete(ctx, "default", "my-task"); err != nil {
		t.Fatalf("ReconcileDelete: %v", err)
	}
	if len(mockSrv.deletedActors) != 1 || mockSrv.deletedActors[0] != "my-task" {
		t.Fatalf("expected actor my-task deleted, got %v", mockSrv.deletedActors)
	}
	if len(mockSrv.deletedTemplates) != 1 || mockSrv.deletedTemplates[0] != "my-task-tmpl-deadbeef" {
		t.Fatalf("expected template deleted, got %v", mockSrv.deletedTemplates)
	}
}
