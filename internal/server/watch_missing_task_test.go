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
	"time"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestWatchTaskMissingTaskNotFound is the regression test for the watch hang
// on a nonexistent task: watching a name the store has never seen must fail
// fast with NotFound (mirroring GetTask's ErrNotFound mapping) instead of
// blocking until the client gives up. `ax watch task <typo>` hung forever
// with only "Watching task..." printed.
// Red on base: the stream stays open and the test times out.
func TestWatchTaskMissingTaskNotFound(t *testing.T) {
	client, cleanup := watchTestClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.WatchTask(ctx, &v1alpha1.WatchTaskRequest{Atespace: "default", Name: "no-such-task"})
	if err != nil {
		t.Fatalf("WatchTask failed: %v", err)
	}
	start := time.Now()
	_, err = stream.Recv()
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("watch on missing task blocked for %v, want fast NotFound", d)
	}
	if status.Code(err) != codes.NotFound {
		t.Fatalf("Recv() err = %v, want NotFound", err)
	}
}
