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

package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// stubWatchStream yields queued responses, then blocks forever — the shape of
// a server that keeps streaming past a terminal phase instead of ending the
// stream.
type stubWatchStream struct {
	resps []*v1alpha1.WatchTaskResponse
	err   error
}

func (s *stubWatchStream) Recv() (*v1alpha1.WatchTaskResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	if len(s.resps) == 0 {
		select {} // block forever
	}
	r := s.resps[0]
	s.resps = s.resps[1:]
	return r, nil
}

func taskResp(phase string) *v1alpha1.WatchTaskResponse {
	return &v1alpha1.WatchTaskResponse{
		Task: &v1alpha1.Task{Status: &v1alpha1.TaskStatus{Phase: phase}},
	}
}

// TestWatchStreamLoopEndsOnTerminating: the CLI's own terminal-phase check
// omitted Terminating (the server's isWatchTerminal includes it), so a
// stream delivering Terminating without ending would hang `ax watch` after
// printing the phase line. Red on base: isTerminalPhase/watchStreamLoop are
// undefined; behaviorally, the old inline check never matched Terminating.
func TestWatchStreamLoopEndsOnTerminating(t *testing.T) {
	stream := &stubWatchStream{resps: []*v1alpha1.WatchTaskResponse{taskResp("Terminating")}}
	done := make(chan error, 1)
	go func() { done <- watchStreamLoop(stream, io.Discard, "default", "t") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("watchStreamLoop err = %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watch loop did not stop on Terminating phase")
	}
}

func TestIsTerminalPhase(t *testing.T) {
	for _, p := range []string{"Running", "Completed", "Failed", "Terminating"} {
		if !isTerminalPhase(p) {
			t.Errorf("isTerminalPhase(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"", "Pending", "Suspended", "Unknown"} {
		if isTerminalPhase(p) {
			t.Errorf("isTerminalPhase(%q) = true, want false", p)
		}
	}
}

// TestWatchStreamLoopNotFound surfaces the server's NotFound (unknown task
// name) as a clear message instead of a wrapped RPC error.
func TestWatchStreamLoopNotFound(t *testing.T) {
	stream := &stubWatchStream{err: status.Error(codes.NotFound, `task "nope" not found`)}
	err := watchStreamLoop(stream, io.Discard, "default", "nope")
	if err == nil || !strings.Contains(err.Error(), `"nope"`) || strings.Contains(err.Error(), "rpc error") {
		t.Fatalf("watchStreamLoop err = %v, want friendly not-found message", err)
	}
}

