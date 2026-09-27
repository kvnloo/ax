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
// limitations in the License.

package substrate

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// retryMaxAttempts bounds how many times a substrate RPC is tried:
	// the initial attempt plus this many retries.
	retryMaxAttempts = 4
	// retryBaseDelay is the backoff before the first retry; it doubles
	// after each subsequent attempt.
	retryBaseDelay = 500 * time.Millisecond
)

// isTransientCode reports whether a gRPC status code is worth retrying: the
// request may not have been applied, or the failure is independent of the
// request's content. Permanent codes (InvalidArgument, NotFound,
// PermissionDenied, AlreadyExists, ...) are returned to the caller
// immediately so real problems surface instead of burning retries.
func isTransientCode(c codes.Code) bool {
	switch c {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted:
		return true
	}
	return false
}

// doWithRetry runs fn, retrying transient gRPC failures with exponential
// backoff. Permanent errors return immediately; context cancellation aborts
// the backoff and is reported as ctx.Err(). Only the reconcile-path RPCs
// use this: they are idempotent (create-or-get / state transitions), and a
// single transient control-plane blip must not mark a task Failed. The
// delete path keeps its own retry policy in the reconciler and is excluded
// so the two policies do not compound.
func doWithRetry(ctx context.Context, op string, fn func(context.Context) error) error {
	delay := retryBaseDelay
	var err error
	for attempt := 1; ; attempt++ {
		err = fn(ctx)
		if err == nil {
			return nil
		}
		if !isTransientCode(status.Code(err)) || attempt >= retryMaxAttempts {
			return err
		}
		slog.Warn("transient substrate error, retrying", "op", op, "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}
