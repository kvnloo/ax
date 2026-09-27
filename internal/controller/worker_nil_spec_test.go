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
	"time"

	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
	"github.com/google/ax/internal/controller"
	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/internal/substrate"
	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// A task with no spec (e.g. a metadata-only manifest through UpdateTask, which
// ValidateTask accepts) must not panic the worker: processEvent dereferences
// task.Spec for the gateway/workspace lookups before Reconcile's own default
// kicks in. A panic here kills the whole controller loop, not just the task.
func TestWorkerNilSpecNoPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	mockSrv := &mockControlServer{}
	grpcServer := grpc.NewServer()
	ateapipb.RegisterControlServer(grpcServer, mockSrv)
	go grpcServer.Serve(lis)
	defer grpcServer.Stop()

	subClient, err := substrate.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to create substrate client: %v", err)
	}
	defer subClient.Close()

	reconciler := controller.NewTaskReconciler(subClient, "default-template", "ax-system")
	reconciler.SecretResolver = noSecrets
	reconciler.WorkspaceReadyTimeout = 200 * time.Millisecond

	memStore := memory.NewStore()
	if err := memStore.SaveTask(ctx, &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "specless", Atespace: "default"},
		// Spec deliberately nil.
	}); err != nil {
		t.Fatalf("failed to save task: %v", err)
	}

	worker := controller.NewWorker(memStore, reconciler, "test-group", "worker-nilspec")
	panicked := make(chan any, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicked <- r
			}
		}()
		_ = worker.Run(ctx)
	}()

	deadline := time.Now().Add(3 * time.Second)
	var finalTask *v1alpha1.Task
	for time.Now().Before(deadline) {
		select {
		case p := <-panicked:
			t.Fatalf("worker panicked on spec-less task: %v", p)
		default:
		}
		tItem, err := memStore.GetTask(ctx, "default", "specless")
		if err == nil && tItem.Status.Phase == "Running" {
			finalTask = tItem
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	select {
	case p := <-panicked:
		t.Fatalf("worker panicked on spec-less task: %v", p)
	default:
	}

	if finalTask == nil {
		t.Fatal("spec-less task did not reconcile to Running in time")
	}
	// Note: only the status is persisted by the worker; the nil Spec lives on
	// in the stored record, which is fine because every consumer (Reconcile,
	// processEvent) now defaults it before dereferencing.
}
