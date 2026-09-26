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
)

// The server contract (internal/model/client.go) is that an empty atespace
// means the default atespace. The Get/Delete handlers already honored it;
// the List handlers passed "" straight to the stores, where "" is the
// cross-atespace wildcard — so `ax --atespace= get tasks` listed every
// atespace instead of just "default".
func TestList_EmptyAtespaceMeansDefault(t *testing.T) {
	srv := server.NewServer(memory.NewStore())
	ctx := context.Background()

	seed := func(kind, atespace, name string) {
		t.Helper()
		meta := &v1alpha1.ObjectMeta{Name: name, Atespace: atespace}
		var err error
		switch kind {
		case "task":
			_, err = srv.UpdateTask(ctx, &v1alpha1.UpdateTaskRequest{Task: &v1alpha1.Task{Metadata: meta}})
		case "gateway":
			_, err = srv.UpdateGateway(ctx, &v1alpha1.UpdateGatewayRequest{Gateway: &v1alpha1.Gateway{Metadata: meta}})
		case "workspace":
			_, err = srv.UpdateWorkspace(ctx, &v1alpha1.UpdateWorkspaceRequest{Workspace: &v1alpha1.Workspace{Metadata: meta}})
		case "model":
			_, err = srv.UpdateModel(ctx, &v1alpha1.UpdateModelRequest{Model: &v1alpha1.Model{Metadata: meta}})
		}
		if err != nil {
			t.Fatalf("seeding %s %s/%s: %v", kind, atespace, name, err)
		}
	}
	for _, kind := range []string{"task", "gateway", "workspace", "model"} {
		seed(kind, "", "d1")     // empty atespace -> stored in "default"
		seed(kind, "other", "o1") // a second atespace to catch wildcarding
	}

	tasks, err := srv.ListTasks(ctx, &v1alpha1.ListTasksRequest{Atespace: ""})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks.Tasks) != 1 || tasks.Tasks[0].Metadata.Name != "d1" {
		t.Fatalf("ListTasks(\"\") = %d tasks, want only the default-atespace one", len(tasks.Tasks))
	}

	gateways, err := srv.ListGateways(ctx, &v1alpha1.ListGatewaysRequest{Atespace: ""})
	if err != nil {
		t.Fatalf("ListGateways: %v", err)
	}
	if len(gateways.Gateways) != 1 || gateways.Gateways[0].Metadata.Name != "d1" {
		t.Fatalf("ListGateways(\"\") = %d gateways, want only the default-atespace one", len(gateways.Gateways))
	}

	workspaces, err := srv.ListWorkspaces(ctx, &v1alpha1.ListWorkspacesRequest{Atespace: ""})
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(workspaces.Workspaces) != 1 || workspaces.Workspaces[0].Metadata.Name != "d1" {
		t.Fatalf("ListWorkspaces(\"\") = %d workspaces, want only the default-atespace one", len(workspaces.Workspaces))
	}

	models, err := srv.ListModels(ctx, &v1alpha1.ListModelsRequest{Atespace: ""})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models.Models) != 1 || models.Models[0].Metadata.Name != "d1" {
		t.Fatalf("ListModels(\"\") = %d models, want only the default-atespace one", len(models.Models))
	}
}
