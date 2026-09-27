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
	"sync"
	"testing"
	"time"

	"github.com/google/ax/internal/server"
	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
)

// watchStream is a minimal grpc.ServerStreamingServer capturing sends.
type watchStream struct {
	grpc.ServerStreamingServer[v1alpha1.WatchTaskResponse]
	ctx   context.Context
	mu    sync.Mutex
	sends []*v1alpha1.WatchTaskResponse
}

func (w *watchStream) Context() context.Context { return w.ctx }

func (w *watchStream) Send(resp *v1alpha1.WatchTaskResponse) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sends = append(w.sends, resp)
	return nil
}

func (w *watchStream) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.sends)
}

func (w *watchStream) last() *v1alpha1.WatchTaskResponse {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sends[len(w.sends)-1]
}

// Deletion is two-phase: MarkTaskDeleting flips the task to Terminating and
// the record disappears once the controller finishes cleanup, with no further
// watch update. Terminating is terminal from a watcher's perspective — no
// phase transition out of it is possible — so the stream must close after
// delivering it instead of hanging until the client gives up.
func TestWatchTaskTerminatingClosesStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := memory.NewStore()
	srv := server.NewServer(st)
	if err := st.SaveTask(ctx, &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: "t1"}}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}

	ws := &watchStream{ctx: ctx}
	done := make(chan error, 1)
	go func() {
		done <- srv.WatchTask(&v1alpha1.WatchTaskRequest{Atespace: "default", Name: "t1"}, ws)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for ws.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ws.count() == 0 {
		t.Fatal("timed out waiting for INITIAL watch response")
	}

	if err := st.MarkTaskDeleting(ctx, "default", "t1"); err != nil {
		t.Fatalf("MarkTaskDeleting: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WatchTask returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WatchTask stream hung after the task reached Terminating; want the stream to close")
	}

	if got := ws.last().Task.GetStatus().GetPhase(); got != v1alpha1.PhaseTerminating {
		t.Fatalf("last watch response phase = %q, want %q", got, v1alpha1.PhaseTerminating)
	}
}
