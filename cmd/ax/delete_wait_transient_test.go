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
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// scriptedGetClient replays a script of GetTask results: each lookup consumes
// the next entry, repeating the last one forever.
type scriptedGetClient struct {
	v1alpha1.AXClient
	mu     sync.Mutex
	script []error
	calls  int
}

func (f *scriptedGetClient) GetTask(ctx context.Context, req *v1alpha1.GetTaskRequest, opts ...grpc.CallOption) (*v1alpha1.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	err := f.script[len(f.script)-1]
	if f.calls <= len(f.script) {
		err = f.script[f.calls-1]
	}
	if err != nil {
		return nil, err
	}
	return &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: req.Name}}, nil
}

// A transient lookup failure (server blip/restart) during the delete wait must
// not turn a successful delete into a reported failure: the wait keeps polling
// until the resource is really gone.
func TestWaitForDeletionToleratesTransientErrors(t *testing.T) {
	client := &scriptedGetClient{script: []error{
		status.Error(codes.Unavailable, "server restarting"),
		status.Error(codes.DeadlineExceeded, "blip"),
		status.Error(codes.NotFound, "gone"),
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := waitForDeletion(ctx, client, v1alpha1.KindTask, "default", "t"); err != nil {
		t.Fatalf("waitForDeletion = %v, want nil (transient Unavailable/DeadlineExceeded aborted the wait)", err)
	}
	if client.calls != 3 {
		t.Fatalf("lookups = %d, want 3 (wait must poll through the transient failures)", client.calls)
	}
}

// A permanent lookup failure still aborts the wait immediately; only the
// transient codes are retried.
func TestWaitForDeletionPermanentErrorAborts(t *testing.T) {
	client := &scriptedGetClient{script: []error{
		status.Error(codes.PermissionDenied, "nope"),
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	err := waitForDeletion(ctx, client, v1alpha1.KindTask, "default", "t")
	if err == nil || !strings.Contains(err.Error(), "checking task") {
		t.Fatalf("waitForDeletion = %v, want the permanent error surfaced", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("waited %v on a permanent error, want an immediate abort", elapsed)
	}
}

// When the resource never disappears, the wait still honors the context
// deadline instead of spinning forever.
func TestWaitForDeletionHonorsContextTimeout(t *testing.T) {
	client := &scriptedGetClient{script: []error{nil}} // task always present
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	err := waitForDeletion(ctx, client, v1alpha1.KindTask, "default", "t")
	if err == nil || !strings.Contains(err.Error(), "timed out waiting") {
		t.Fatalf("waitForDeletion = %v, want a timeout error", err)
	}
}
