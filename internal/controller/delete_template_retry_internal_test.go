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

package controller

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDeleteTemplateWithRetry_HonorsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := deleteTemplateWithRetry(ctx, func(ctx context.Context) error {
		calls++
		return status.Error(codes.Aborted, "still deleting")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("delete called %d times after cancellation, want 1 (base loop burns all 5 attempts)", calls)
	}
}

func TestDeleteTemplateWithRetry_RetriesTransient(t *testing.T) {
	calls := 0
	err := deleteTemplateWithRetry(context.Background(), func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return status.Error(codes.Aborted, "still deleting")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	if calls != 3 {
		t.Fatalf("delete called %d times, want 3", calls)
	}
}

func TestDeleteTemplateWithRetry_NotFoundIsSuccess(t *testing.T) {
	calls := 0
	err := deleteTemplateWithRetry(context.Background(), func(ctx context.Context) error {
		calls++
		return status.Error(codes.NotFound, "already gone")
	})
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	if calls != 1 {
		t.Fatalf("delete called %d times, want 1", calls)
	}
}

func TestDeleteTemplateWithRetry_ExhaustsAttempts(t *testing.T) {
	calls := 0
	err := deleteTemplateWithRetry(context.Background(), func(ctx context.Context) error {
		calls++
		return status.Error(codes.Aborted, "stuck")
	})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("got %v, want Aborted", err)
	}
	if calls != 5 {
		t.Fatalf("delete called %d times, want 5", calls)
	}
}
