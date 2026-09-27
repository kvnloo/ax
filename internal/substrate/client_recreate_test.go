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

package substrate_test

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
	"github.com/google/ax/internal/substrate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// recreateMock simulates an actor stuck in DELETING: the first CreateActor
// reports AlreadyExists, GetActor reports DELETING, and the retry-time
// CreateActor outcome is scripted via createErr.
type recreateMock struct {
	ateapipb.UnimplementedControlServer
	createCalls int
	createErr   error
}

func (m *recreateMock) CreateActor(ctx context.Context, req *ateapipb.CreateActorRequest) (*ateapipb.Actor, error) {
	m.createCalls++
	if m.createCalls == 1 {
		return nil, status.Error(codes.AlreadyExists, "exists")
	}
	if m.createErr != nil {
		return nil, m.createErr
	}
	return &ateapipb.Actor{Metadata: &ateapipb.ResourceMetadata{Name: "a1"}}, nil
}

func (m *recreateMock) GetActor(ctx context.Context, req *ateapipb.GetActorRequest) (*ateapipb.Actor, error) {
	return &ateapipb.Actor{
		Metadata: &ateapipb.ResourceMetadata{Name: "a1"},
		Status:   &ateapipb.ActorStatus{State: ateapipb.ActorState_ACTOR_STATE_DELETING},
	}, nil
}

func dialRecreateMock(t *testing.T, mock *recreateMock) *substrate.Client {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	t.Cleanup(func() { lis.Close() })
	grpcServer := grpc.NewServer()
	ateapipb.RegisterControlServer(grpcServer, mock)
	go grpcServer.Serve(lis)
	t.Cleanup(grpcServer.Stop)

	client, err := substrate.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to create substrate client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

// A genuine creation failure while waiting out a deleting actor must surface
// the real error, not "still deleting; please retry".
func TestEnsureActor_RecreateFailureSurfacesRealError(t *testing.T) {
	ctx := context.Background()
	mock := &recreateMock{createErr: status.Error(codes.InvalidArgument, "bad template ref")}
	client := dialRecreateMock(t, mock)

	_, err := client.EnsureActor(ctx, "ns", "a1", "tmpl-ns", "tmpl")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "bad template ref") {
		t.Errorf("real creation error was masked: %v", err)
	}
	if strings.Contains(err.Error(), "still deleting") {
		t.Errorf("misleading 'still deleting' message for a real failure: %v", err)
	}
}

// Control: a successful recreate after deletion still works.
func TestEnsureActor_RecreateAfterDeletingSucceeds(t *testing.T) {
	ctx := context.Background()
	mock := &recreateMock{}
	client := dialRecreateMock(t, mock)

	actor, err := client.EnsureActor(ctx, "ns", "a1", "tmpl-ns", "tmpl")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if actor.GetMetadata().GetName() != "a1" {
		t.Errorf("expected actor a1, got %q", actor.GetMetadata().GetName())
	}
}
