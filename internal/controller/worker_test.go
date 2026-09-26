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
	"errors"
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

func TestWorkerReconciliation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Start in-process mock Substrate server
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

	// 2. Substrate client
	subClient, err := substrate.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to create substrate client: %v", err)
	}
	defer subClient.Close()

	reconciler := controller.NewTaskReconciler(subClient, "default-template", "ax-system")
	reconciler.SecretResolver = noSecrets
	reconciler.WorkspaceReadyTimeout = 200 * time.Millisecond

	// 3. In-memory store
	memStore := memory.NewStore()

	// Pre-create gateway
	_ = memStore.SaveGateway(ctx, &v1alpha1.Gateway{
		Metadata: &v1alpha1.ObjectMeta{Name: "default-gw", Atespace: "default"},
		Spec: &v1alpha1.GatewaySpec{
			Egress: &v1alpha1.EgressConfig{
				Allowlist: &v1alpha1.EgressAllowlist{
					Hosts: []*v1alpha1.HostRule{{Host: "api.openai.com"}},
				},
			},
		},
	})

	// Save task
	task := &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "worker-task", Atespace: "default"},
		Spec: &v1alpha1.TaskSpec{
			Gateway: &v1alpha1.GatewayRef{Name: "default-gw"},
			Image:   "ghrc.io/test/img",
		},
	}
	if err := memStore.SaveTask(ctx, task); err != nil {
		t.Fatalf("failed to save task: %v", err)
	}

	// 4. Start the worker in the background
	worker := controller.NewWorker(memStore, reconciler, "test-group", "worker-1")
	go func() {
		_ = worker.Run(ctx)
	}()

	// 5. Poll store until task reaches "Running" phase
	deadline := time.Now().Add(3 * time.Second)
	var finalTask *v1alpha1.Task
	for time.Now().Before(deadline) {
		tItem, err := memStore.GetTask(ctx, "default", "worker-task")
		if err == nil && tItem.Status.Phase == "Running" {
			finalTask = tItem
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if finalTask == nil {
		t.Fatalf("task did not transition to Running phase in time")
	}

	if finalTask.Status.Actor != "worker-task" {
		t.Errorf("expected actor 'worker-task', got %q", finalTask.Status.Actor)
	}
	if finalTask.Status.WorkerIp != "10.244.1.42" {
		t.Errorf("expected worker IP '10.244.1.42', got %q", finalTask.Status.WorkerIp)
	}
}

func TestWorkerDeletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	mockSrv := &mockControlServer{actorTemplates: map[string]bool{"doomed-tmpl-0a1b2c3d": true}}
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
	task := &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "doomed", Atespace: "default"},
		Spec:     &v1alpha1.TaskSpec{Image: "ghcr.io/test/img"},
		Status:   &v1alpha1.TaskStatus{Phase: "Running", Actor: "doomed"},
	}
	if err := memStore.SaveTask(ctx, task); err != nil {
		t.Fatalf("failed to save task: %v", err)
	}
	// Drain the reconcile event SaveTask published so only the delete is processed.
	drain, _ := memStore.Subscribe(ctx, "drain", "drain")
	drainCtx, drainCancel := context.WithTimeout(ctx, time.Second)
	_, _ = drain.Next(drainCtx)
	drainCancel()

	if err := memStore.MarkTaskDeleting(ctx, "default", "doomed"); err != nil {
		t.Fatalf("MarkTaskDeleting failed: %v", err)
	}
	marked, err := memStore.GetTask(ctx, "default", "doomed")
	if err != nil {
		t.Fatalf("GetTask after mark failed: %v", err)
	}
	if marked.Status.Phase != v1alpha1.PhaseTerminating {
		t.Fatalf("expected phase Terminating, got %q", marked.Status.Phase)
	}

	worker := controller.NewWorker(memStore, reconciler, "test-group", "worker-1")
	go func() { _ = worker.Run(ctx) }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := memStore.GetTask(ctx, "default", "doomed"); err != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := memStore.GetTask(ctx, "default", "doomed"); err == nil {
		t.Fatalf("expected task record to be removed after cleanup")
	}
	if len(mockSrv.deletedActors) != 1 || mockSrv.deletedActors[0] != "doomed" {
		t.Errorf("expected actor 'doomed' deleted, got %v", mockSrv.deletedActors)
	}
	if len(mockSrv.deletedTemplates) != 1 || mockSrv.deletedTemplates[0] != "doomed-tmpl-0a1b2c3d" {
		t.Errorf("expected template deleted, got %v", mockSrv.deletedTemplates)
	}
}

// TestWorkerTerminatingReconcileCompletesDeletion is the regression test for
// the resurrection defect: a "reconcile" event arriving for a Terminating
// task (e.g. an update racing the delete — every SaveTask publishes one) ran
// the full Reconcile path, calling ResumeActor on an actor that was being
// torn down. The delete path must win regardless of the event action.
// Red on base (old code resumes the actor a second time and flips the task
// back to Running).
func TestWorkerTerminatingReconcileCompletesDeletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	// Fail actor deletion so the record stays Terminating after the delete
	// event, letting the racing reconcile event observe that phase.
	mockSrv := &mockControlServer{deleteActorErr: errors.New("injected delete failure")}
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
	task := &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "resurrect-me", Atespace: "default"},
		Spec:     &v1alpha1.TaskSpec{Image: "ghcr.io/test/img"},
	}
	if err := memStore.SaveTask(ctx, task); err != nil {
		t.Fatalf("failed to save task: %v", err)
	}

	worker := controller.NewWorker(memStore, reconciler, "test-group", "worker-1")
	go func() { _ = worker.Run(ctx) }()

	// Wait for the initial reconcile to bring the task to Running.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if tItem, err := memStore.GetTask(ctx, "default", "resurrect-me"); err == nil && tItem.Status.Phase == "Running" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got, _ := memStore.GetTask(ctx, "default", "resurrect-me"); got.Status.Phase != "Running" {
		t.Fatalf("task did not reach Running, got %q", got.Status.Phase)
	}
	if n := mockSrv.resumedCount(); n != 1 {
		t.Fatalf("expected 1 resume from initial reconcile, got %d", n)
	}

	// Mark deleting: the delete event's cleanup fails (injected), so the
	// record stays Terminating.
	if err := memStore.MarkTaskDeleting(ctx, "default", "resurrect-me"); err != nil {
		t.Fatalf("MarkTaskDeleting failed: %v", err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && mockSrv.deletedActorCount() < 1 {
		time.Sleep(50 * time.Millisecond)
	}
	if n := mockSrv.deletedActorCount(); n < 1 {
		t.Fatalf("delete event was not processed in time")
	}

	// A racing update publishes a "reconcile" event for the Terminating task.
	racing, err := memStore.GetTask(ctx, "default", "resurrect-me")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if racing.Status.Phase != v1alpha1.PhaseTerminating {
		t.Fatalf("expected Terminating after failed cleanup, got %q", racing.Status.Phase)
	}
	if err := memStore.SaveTask(ctx, racing); err != nil {
		t.Fatalf("SaveTask failed: %v", err)
	}

	// Let the worker process the racing reconcile event.
	time.Sleep(time.Second)

	if n := mockSrv.resumedCount(); n != 1 {
		t.Errorf("reconcile event for a Terminating task resumed the actor: %d resumes, want 1", n)
	}
	if got, _ := memStore.GetTask(ctx, "default", "resurrect-me"); got.Status.Phase != v1alpha1.PhaseTerminating {
		t.Errorf("task left Terminating by racing reconcile, got %q", got.Status.Phase)
	}
}
