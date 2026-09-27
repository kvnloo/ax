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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

// watchForwarderAlive reports whether a WatchTask forwarder goroutine still
// exists. It matches the forwarder by its stack frame instead of counting
// goroutines, so background runtime activity cannot skew the result.
func watchForwarderAlive() bool {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	for _, g := range strings.Split(string(buf[:n]), "\n\n") {
		if strings.Contains(g, "WatchTask.func1") {
			return true
		}
	}
	return false
}

// TestWatchTaskCloserReleasesBlockedForwarder proves the watch forwarder
// goroutine cannot get stuck on a full channel: with nobody reading the
// watch channel, closing the watcher must still release the goroutine.
// (Draining the channel is deliberately avoided: on the buggy version a
// single read would unblock the parked send and hide the leak.)
func TestWatchTaskCloserReleasesBlockedForwarder(t *testing.T) {
	ctx := context.Background()

	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer client.Close()

	s := NewStore(client, Options{})

	if watchForwarderAlive() {
		t.Fatal("forwarder already running before WatchTask")
	}

	ch, closer, err := s.WatchTask(ctx, "default", "leak-task")
	if err != nil {
		t.Fatalf("WatchTask failed: %v", err)
	}

	// Burst past the 10-slot channel buffer without reading ch, so the
	// forwarder ends up parked on a blocked send.
	payload := `{"metadata":{"name":"leak-task","atespace":"default"},"status":{"phase":"Running"}}`
	channel := s.taskPubSubChannel("default", "leak-task")
	for i := 0; i < 25; i++ {
		if err := client.Publish(ctx, channel, payload).Err(); err != nil {
			t.Fatalf("publish %d failed: %v", i, err)
		}
	}
	time.Sleep(time.Second)
	if got := len(ch); got != 10 {
		t.Fatalf("expected the forwarder to be parked with a full 10-slot buffer, got %d", got)
	}
	if !watchForwarderAlive() {
		t.Fatal("forwarder goroutine missing before closer.Close")
	}

	if err := closer.Close(); err != nil {
		t.Fatalf("closer.Close failed: %v", err)
	}

	// Let any cleanup run, then check the forwarder is gone.
	time.Sleep(time.Second)
	if watchForwarderAlive() {
		t.Fatal("forwarder goroutine leaked: still alive after closer.Close() with no reader")
	}
}
