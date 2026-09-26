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
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/ax/internal/server"
	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// watchTestClient starts a server over a memory store and returns a gRPC
// client for it.
func watchTestClient(t *testing.T) (v1alpha1.AXClient, func()) {
	t.Helper()
	memStore := memory.NewStore()
	srv := server.NewServer(memStore)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	httpServer := &http.Server{Handler: srv.Handler()}
	httpServer.Protocols = new(http.Protocols)
	httpServer.Protocols.SetHTTP1(true)
	httpServer.Protocols.SetUnencryptedHTTP2(true)
	go func() { _ = httpServer.Serve(ln) }()

	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial gRPC: %v", err)
	}

	return v1alpha1.NewAXClient(conn), func() {
		conn.Close()
		httpServer.Close()
		ln.Close()
	}
}

// recvUntilDone drains a watch stream in the background and reports the
// first terminal condition: stream end (nil) or stream error.
func recvUntilDone(stream v1alpha1.AX_WatchTaskClient) <-chan error {
	done := make(chan error, 1)
	go func() {
		for {
			if _, err := stream.Recv(); err != nil {
				done <- err
				return
			}
		}
	}()
	return done
}

// TestWatchTaskEndsOnDelete is the regression test for the watch hang on
// task deletion: deletion is two-phase (Terminating, then the record
// disappears) and no further watch notification is published on removal, so
// a watch that is not ended at Terminating blocks until the client gives
// up. Red on base: the stream stays open and the test times out.
func TestWatchTaskEndsOnDelete(t *testing.T) {
	client, cleanup := watchTestClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := client.UpdateTask(ctx, &v1alpha1.UpdateTaskRequest{Task: &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "watch-del"},
		Spec:     &v1alpha1.TaskSpec{Image: "alpine"},
	}}); err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	stream, err := client.WatchTask(ctx, &v1alpha1.WatchTaskRequest{Atespace: "default", Name: "watch-del"})
	if err != nil {
		t.Fatalf("WatchTask failed: %v", err)
	}
	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("expected INITIAL message, got: %v", err)
	}
	if first.Action != "INITIAL" {
		t.Fatalf("expected INITIAL action, got %q", first.Action)
	}
	done := recvUntilDone(stream)

	if _, err := client.DeleteTask(ctx, &v1alpha1.DeleteTaskRequest{Atespace: "default", Name: "watch-del"}); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected clean stream end after delete, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WatchTask stream did not terminate after the task was deleted")
	}
}

// TestWatchTaskEndsOnAlreadyTerminating covers the same hang when the watch
// starts after deletion was requested: INITIAL is already Terminating.
func TestWatchTaskEndsOnAlreadyTerminating(t *testing.T) {
	client, cleanup := watchTestClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := client.UpdateTask(ctx, &v1alpha1.UpdateTaskRequest{Task: &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "watch-term"},
		Spec:     &v1alpha1.TaskSpec{Image: "alpine"},
	}}); err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}
	if _, err := client.DeleteTask(ctx, &v1alpha1.DeleteTaskRequest{Atespace: "default", Name: "watch-term"}); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	stream, err := client.WatchTask(ctx, &v1alpha1.WatchTaskRequest{Atespace: "default", Name: "watch-term"})
	if err != nil {
		t.Fatalf("WatchTask failed: %v", err)
	}
	done := recvUntilDone(stream)

	select {
	case err := <-done:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected clean stream end on terminating task, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WatchTask stream did not terminate on an already-terminating task")
	}
}
