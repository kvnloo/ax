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
	"testing"
	"time"

	"github.com/google/ax/pkg/apis/v1alpha1"
	goredis "github.com/redis/go-redis/v9"
)

func taskPayload(t *testing.T, name, phase string) string {
	t.Helper()
	data, err := jsonMarshalOpts.Marshal(&v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: name},
		Status:   &v1alpha1.TaskStatus{Phase: phase},
	})
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}
	return string(data)
}

// TestWatchForwardLoopDoesNotBlockOnFullChannel is the regression test for
// the goroutine leak: the old loop did a blocking send on the size-10
// buffered channel, so once a consumer stopped reading (e.g. the server's
// WatchTask returned after a terminal phase) the goroutine pinned itself
// forever on the 11th update. The new loop must return promptly even with
// no reader and a full buffer. Red on base (watchForwardLoop is new).
func TestWatchForwardLoopDoesNotBlockOnFullChannel(t *testing.T) {
	msgCh := make(chan *goredis.Message)
	ch := make(chan *v1alpha1.Task, 10)
	// Fill the buffer with no reader, like a consumer that went away.
	for i := 0; i < 10; i++ {
		ch <- &v1alpha1.Task{}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		watchForwardLoop(msgCh, ch)
	}()

	// Feed more updates than the buffer holds, then close the source.
	go func() {
		for i := 0; i < 5; i++ {
			msgCh <- &goredis.Message{Payload: taskPayload(t, "t", "Running")}
		}
		close(msgCh)
	}()

	select {
	case <-done:
		// The loop exited and closed ch despite the full buffer and no reader.
	case <-time.After(3 * time.Second):
		t.Fatal("watchForwardLoop blocked forever on a full channel with no reader (goroutine leak)")
	}
	// Drain the 10 prefilled tasks; the 5 new updates must have been dropped,
	// not buffered, and ch must then be closed.
	drained := 0
	for range ch {
		drained++
	}
	if drained != 10 {
		t.Fatalf("drained %d tasks, want exactly the 10 prefilled (new updates must be dropped, not delivered)", drained)
	}
}

// TestWatchForwardLoopDeliversInOrder verifies the normal path: with a live
// reader every decodable payload arrives in order, undecodable payloads are
// skipped, and closing the source closes ch.
func TestWatchForwardLoopDeliversInOrder(t *testing.T) {
	msgCh := make(chan *goredis.Message)
	ch := make(chan *v1alpha1.Task, 10)
	go watchForwardLoop(msgCh, ch)

	msgCh <- &goredis.Message{Payload: taskPayload(t, "t1", "Pending")}
	msgCh <- &goredis.Message{Payload: "not-json{"}
	msgCh <- &goredis.Message{Payload: taskPayload(t, "t2", "Running")}
	msgCh <- &goredis.Message{Payload: taskPayload(t, "t3", "Completed")}
	close(msgCh)

	var got []string
	for task := range ch {
		got = append(got, task.Metadata.Name)
	}
	want := []string{"t1", "t2", "t3"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
