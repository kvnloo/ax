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
	"net/http"
	"testing"
	"time"

	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
	"github.com/google/ax/internal/controller"
	"github.com/google/ax/internal/substrate"
	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func (m *mockControlServer) GetActorTemplate(ctx context.Context, req *ateapipb.GetActorTemplateRequest) (*ateapipb.ActorTemplate, error) {
	name := req.GetActorTemplate().GetName()
	if m.actorTemplates != nil && m.actorTemplates[name] {
		return &ateapipb.ActorTemplate{Metadata: &ateapipb.ResourceMetadata{Name: name}}, nil
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func (m *mockControlServer) CreateActorTemplate(ctx context.Context, req *ateapipb.CreateActorTemplateRequest) (*ateapipb.ActorTemplate, error) {
	name := req.GetActorTemplate().GetMetadata().GetName()
	if m.actorTemplates == nil {
		m.actorTemplates = map[string]bool{}
	}
	m.actorTemplates[name] = true
	return &ateapipb.ActorTemplate{Metadata: &ateapipb.ResourceMetadata{Name: name}}, nil
}

// Reconciling the same task twice must not mint a second ActorTemplate when
// only the task's status changed. The template name is supposed to digest the
// task's desired state; status-only churn (condition timestamps, phase flips)
// must not produce a new template per reconcile.
func TestTaskReconciler_TemplateNameStableAcrossStatusChanges(t *testing.T) {
	ctx := context.Background()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	httpLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen http: %v", err)
	}
	defer httpLis.Close()

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	httpServer := &http.Server{Handler: httpMux}
	go httpServer.Serve(httpLis)
	defer httpServer.Close()

	mockSrv := &mockControlServer{actorTemplates: map[string]bool{}}
	mockSrv.workerIP = httpLis.Addr().String()
	grpcServer := grpc.NewServer()
	ateapipb.RegisterControlServer(grpcServer, mockSrv)
	go grpcServer.Serve(lis)
	defer grpcServer.Stop()

	client, err := substrate.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to create substrate client: %v", err)
	}
	defer client.Close()

	reconciler := controller.NewTaskReconciler(client, "test-template", "ax-system")
	reconciler.SecretResolver = noSecrets
	reconciler.WorkspaceReadyTimeout = 200 * time.Millisecond

	task := &v1alpha1.Task{
		ApiVersion: v1alpha1.APIVersion,
		Kind:       v1alpha1.KindTask,
		Metadata: &v1alpha1.ObjectMeta{
			Name:     "stable-template-task",
			Atespace: "default",
		},
		Spec: &v1alpha1.TaskSpec{
			Image: "ghrc.io/my-org/my-image",
			Env: []*v1alpha1.EnvVar{
				{Name: "FOO", Value: "bar"},
			},
		},
	}

	if _, err := reconciler.Reconcile(ctx, task, nil); err != nil {
		t.Fatalf("first Reconcile failed: %v", err)
	}
	if len(mockSrv.actorTemplates) != 1 {
		t.Fatalf("expected 1 actor template after first reconcile, got %d", len(mockSrv.actorTemplates))
	}

	// Second reconcile of the same task object: its status now carries the
	// first pass's timestamps, phase, and worker IP, but the spec is unchanged.
	if _, err := reconciler.Reconcile(ctx, task, nil); err != nil {
		t.Fatalf("second Reconcile failed: %v", err)
	}
	if len(mockSrv.actorTemplates) != 1 {
		t.Errorf("status-only change minted a new ActorTemplate: have %d templates, want 1", len(mockSrv.actorTemplates))
		for name := range mockSrv.actorTemplates {
			t.Logf("template: %s", name)
		}
	}
}
