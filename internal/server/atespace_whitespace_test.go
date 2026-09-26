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

// A whitespace-only atespace (e.g. `ax -a=" " get tasks`) used to sail
// through the "" → "default" normalization: the handlers passed " " to the
// stores, where it exact-matches nothing, and the call silently returned
// empty results instead of the default atespace's resources. Every handler
// must treat whitespace-only like empty.
func TestWhitespaceAtespaceMeansDefault(t *testing.T) {
	srv := server.NewServer(memory.NewStore())
	ctx := context.Background()

	if _, err := srv.UpdateTask(ctx, &v1alpha1.UpdateTaskRequest{Task: &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "d1", Atespace: ""},
	}}); err != nil {
		t.Fatalf("seeding task: %v", err)
	}
	if _, err := srv.UpdateGateway(ctx, &v1alpha1.UpdateGatewayRequest{Gateway: &v1alpha1.Gateway{
		Metadata: &v1alpha1.ObjectMeta{Name: "g1", Atespace: ""},
	}}); err != nil {
		t.Fatalf("seeding gateway: %v", err)
	}

	tasks, err := srv.ListTasks(ctx, &v1alpha1.ListTasksRequest{Atespace: "   "})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks.Tasks) != 1 || tasks.Tasks[0].Metadata.Name != "d1" {
		t.Fatalf(`ListTasks("   ") = %d tasks, want only the default-atespace one`, len(tasks.Tasks))
	}

	gws, err := srv.ListGateways(ctx, &v1alpha1.ListGatewaysRequest{Atespace: "\t"})
	if err != nil {
		t.Fatalf("ListGateways: %v", err)
	}
	if len(gws.Gateways) != 1 || gws.Gateways[0].Metadata.Name != "g1" {
		t.Fatalf(`ListGateways("\t") = %d gateways, want only the default-atespace one`, len(gws.Gateways))
	}

	task, err := srv.GetTask(ctx, &v1alpha1.GetTaskRequest{Atespace: " ", Name: "d1"})
	if err != nil || task.Metadata.Name != "d1" {
		t.Fatalf(`GetTask(" ", "d1") = %v, %v, want the default-atespace task`, task, err)
	}

	wss, err := srv.ListWorkspaces(ctx, &v1alpha1.ListWorkspacesRequest{Atespace: " "})
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(wss.Workspaces) != 0 {
		t.Fatalf(`ListWorkspaces(" ") = %d workspaces, want 0 (none seeded)`, len(wss.Workspaces))
	}

	models, err := srv.ListModels(ctx, &v1alpha1.ListModelsRequest{Atespace: " "})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models.Models) != 0 {
		t.Fatalf(`ListModels(" ") = %d models, want 0 (none seeded)`, len(models.Models))
	}
}

// Whitespace-only metadata.atespace on Update must also land in "default"
// (defaultMetadata), not create an invisible " " atespace.
func TestUpdate_WhitespaceAtespaceSavedAsDefault(t *testing.T) {
	srv := server.NewServer(memory.NewStore())
	ctx := context.Background()

	updated, err := srv.UpdateModel(ctx, &v1alpha1.UpdateModelRequest{Model: &v1alpha1.Model{
		Metadata: &v1alpha1.ObjectMeta{Name: "m1", Atespace: "  "},
	}})
	if err != nil {
		t.Fatalf("UpdateModel: %v", err)
	}
	if got := updated.Metadata.Atespace; got != "default" {
		t.Fatalf("UpdateModel whitespace atespace saved as %q, want \"default\"", got)
	}

	// And the ordinary lookup finds it under "default".
	got, err := srv.GetModel(ctx, &v1alpha1.GetModelRequest{Atespace: "default", Name: "m1"})
	if err != nil || got.Metadata.Name != "m1" {
		t.Fatalf(`GetModel("default", "m1") = %v, %v, want the saved model`, got, err)
	}
}

// Non-whitespace atespaces must pass through untouched.
func TestAtespaceNonEmptyPassesThrough(t *testing.T) {
	srv := server.NewServer(memory.NewStore())
	ctx := context.Background()

	if _, err := srv.UpdateTask(ctx, &v1alpha1.UpdateTaskRequest{Task: &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "p1", Atespace: "prod"},
	}}); err != nil {
		t.Fatalf("seeding task: %v", err)
	}

	tasks, err := srv.ListTasks(ctx, &v1alpha1.ListTasksRequest{Atespace: "prod"})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks.Tasks) != 1 || tasks.Tasks[0].Metadata.Name != "p1" {
		t.Fatalf(`ListTasks("prod") = %d tasks, want the prod one`, len(tasks.Tasks))
	}
}
