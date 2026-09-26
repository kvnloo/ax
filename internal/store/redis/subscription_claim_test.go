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

package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// stubStreamClient scripts XREADGROUP/XAUTOCLAIM results so the subscription's
// reclaim logic is testable without a live server. The transport is faked;
// read()/claimStale()/claimDue() under test are the real production code.
type stubStreamClient struct {
	readErr   error
	readMsgs  []redis.XMessage
	claimErr  error
	claimMsgs []redis.XMessage

	readCalls  int
	claimCalls int
}

func (s *stubStreamClient) XReadGroup(ctx context.Context, args *redis.XReadGroupArgs) *redis.XStreamSliceCmd {
	s.readCalls++
	cmd := redis.NewXStreamSliceCmd(ctx)
	if s.readErr != nil {
		cmd.SetErr(s.readErr)
		return cmd
	}
	cmd.SetVal([]redis.XStream{{Stream: args.Streams[0], Messages: s.readMsgs}})
	return cmd
}

func (s *stubStreamClient) XAutoClaim(ctx context.Context, args *redis.XAutoClaimArgs) *redis.XAutoClaimCmd {
	s.claimCalls++
	cmd := redis.NewXAutoClaimCmd(ctx)
	if s.claimErr != nil {
		cmd.SetErr(s.claimErr)
		return cmd
	}
	cmd.SetVal(s.claimMsgs, "0-0")
	return cmd
}

func xmsg(id, name string) redis.XMessage {
	return redis.XMessage{
		ID:     id,
		Values: map[string]interface{}{"atespace": "default", "name": name, "action": "reconcile"},
	}
}

func testSub(stub *stubStreamClient) *subscription {
	return &subscription{
		store:    &Store{opts: Options{StreamName: "ax:stream:tasks", ClaimMinIdle: time.Minute}},
		client:   stub,
		group:    "g",
		consumer: "c",
	}
}

// TestBusyReadReclaimsStaleEntries is the regression test for the busy-stream
// failover hole: before the fix, claimStale ran only when XREADGROUP returned
// Nil (an idle poll), so under sustained traffic a dead consumer's pending
// entries idled forever and the task wedged. A busy read must sweep the
// pending list (throttled) and deliver the older claimed entries first.
// Red on base: base never calls XAutoClaim on a busy read (0 calls, only the
// new event returned).
func TestBusyReadReclaimsStaleEntries(t *testing.T) {
	stub := &stubStreamClient{
		readMsgs:  []redis.XMessage{xmsg("2-0", "new-task")},
		claimMsgs: []redis.XMessage{xmsg("1-0", "orphaned-task")},
	}
	sub := testSub(stub) // lastClaim zero: sweep is due immediately

	events, err := sub.read(context.Background())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if stub.claimCalls != 1 {
		t.Fatalf("expected 1 XAUTOCLAIM sweep on a busy read, got %d", stub.claimCalls)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events (claimed + new), got %d", len(events))
	}
	if events[0].Name != "orphaned-task" || events[1].Name != "new-task" {
		t.Fatalf("claimed (older) entries must come first, got %q then %q", events[0].Name, events[1].Name)
	}
}

// TestBusyReadSweepIsThrottled pins the one-sweep-per-ClaimMinIdle bound: a
// loaded stream must not pay an XAUTOCLAIM round-trip on every read.
func TestBusyReadSweepIsThrottled(t *testing.T) {
	stub := &stubStreamClient{
		readMsgs:  []redis.XMessage{xmsg("2-0", "new-task")},
		claimMsgs: []redis.XMessage{xmsg("1-0", "orphaned-task")},
	}
	sub := testSub(stub)
	sub.lastClaim = time.Now() // swept a moment ago

	events, err := sub.read(context.Background())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if stub.claimCalls != 0 {
		t.Fatalf("expected no XAUTOCLAIM sweep inside the throttle window, got %d calls", stub.claimCalls)
	}
	if len(events) != 1 || events[0].Name != "new-task" {
		t.Fatalf("expected only the new event, got %+v", events)
	}
}

// TestBusyReadClaimFailureDoesNotWedge pins the swallow policy: an old server
// without XAUTOCLAIM must not wedge the worker under load either.
func TestBusyReadClaimFailureDoesNotWedge(t *testing.T) {
	stub := &stubStreamClient{
		readMsgs: []redis.XMessage{xmsg("2-0", "new-task")},
		claimErr: errors.New("ERR unknown command 'xautoclaim'"),
	}
	sub := testSub(stub)

	events, err := sub.read(context.Background())
	if err != nil {
		t.Fatalf("claim failure must not surface as a read error, got: %v", err)
	}
	if len(events) != 1 || events[0].Name != "new-task" {
		t.Fatalf("expected the new event despite the claim failure, got %+v", events)
	}
}
