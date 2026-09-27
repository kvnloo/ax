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

package substrate

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsTransientCode(t *testing.T) {
	transient := []codes.Code{codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted}
	for _, c := range transient {
		if !isTransientCode(c) {
			t.Errorf("code %v should be transient", c)
		}
	}
	permanent := []codes.Code{
		codes.OK, codes.InvalidArgument, codes.NotFound, codes.AlreadyExists,
		codes.PermissionDenied, codes.FailedPrecondition, codes.OutOfRange,
		codes.Unimplemented, codes.Internal, codes.DataLoss, codes.Unauthenticated,
	}
	for _, c := range permanent {
		if isTransientCode(c) {
			t.Errorf("code %v should NOT be transient", c)
		}
	}
}

func TestDoWithRetry_TransientThenSuccess(t *testing.T) {
	calls := 0
	err := doWithRetry(context.Background(), "test", func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return status.Error(codes.Unavailable, "control plane blip")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
}

func TestDoWithRetry_PermanentNotRetried(t *testing.T) {
	calls := 0
	err := doWithRetry(context.Background(), "test", func(ctx context.Context) error {
		calls++
		return status.Error(codes.InvalidArgument, "bad request")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("permanent error must not be retried, got %d attempts", calls)
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestDoWithRetry_ExhaustsAttempts(t *testing.T) {
	calls := 0
	err := doWithRetry(context.Background(), "test", func(ctx context.Context) error {
		calls++
		return status.Error(codes.Unavailable, "still down")
	})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if calls != retryMaxAttempts {
		t.Fatalf("expected %d attempts, got %d", retryMaxAttempts, calls)
	}
}

func TestDoWithRetry_ContextCancelAbortsBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	done := make(chan error, 1)
	go func() {
		done <- doWithRetry(ctx, "test", func(ctx context.Context) error {
			calls++
			return status.Error(codes.Unavailable, "down")
		})
	}()
	// Let the first attempt fail and the backoff begin, then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("doWithRetry did not return after context cancellation")
	}
	if calls != 1 {
		t.Fatalf("expected 1 attempt before cancel, got %d", calls)
	}
}
