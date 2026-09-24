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
	"fmt"
	"net"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/encoding/protojson"
)

// retryHook exercises SaveTask's actual retry loop without a Redis server.
// It scripts a failed EXEC followed by a new read; it does not test Redis's
// transaction isolation or notification delivery.
type retryHook struct {
	reads      int
	secondRead string
	secondErr  error
	payloads   [][]byte
}

func (h *retryHook) DialHook(redis.DialHook) redis.DialHook {
	return func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("unexpected network access in retry test")
	}
}

func (h *retryHook) ProcessHook(redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		switch cmd.Name() {
		case "watch", "unwatch":
			cmd.(*redis.StatusCmd).SetVal("OK")
			return nil
		case "get":
			h.reads++
			if h.reads == 1 {
				cmd.(*redis.StringCmd).SetVal(`{"status":{"phase":"Running","id":"old-runtime"}}`)
				return nil
			}
			cmd.(*redis.StringCmd).SetVal(h.secondRead)
			return h.secondErr
		default:
			return fmt.Errorf("unexpected command %q", cmd.Name())
		}
	}
}

func (h *retryHook) ProcessPipelineHook(redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(_ context.Context, cmds []redis.Cmder) error {
		var payload []byte
		for _, cmd := range cmds {
			if cmd.Name() == "set" {
				data, ok := cmd.Args()[2].([]byte)
				if !ok {
					return fmt.Errorf("unexpected SET payload type %T", cmd.Args()[2])
				}
				payload = append([]byte(nil), data...)
			}
		}
		if payload == nil {
			return errors.New("transaction did not include a task SET")
		}
		h.payloads = append(h.payloads, payload)
		if len(h.payloads) == 1 {
			return redis.TxFailedErr
		}
		return nil
	}
}

func TestSaveTaskRetryStatus(t *testing.T) {
	readErr := errors.New("second read failed")
	for _, tc := range []struct {
		name       string
		secondRead string
		secondErr  error
		phase      string
		id         string
	}{
		{name: "record disappeared", secondErr: redis.Nil, phase: "Pending"},
		{name: "newer status", secondRead: `{"status":{"phase":"Completed","id":"new-runtime"}}`, phase: "Completed", id: "new-runtime"},
		{name: "read failed", secondErr: readErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hook := &retryHook{secondRead: tc.secondRead, secondErr: tc.secondErr}
			client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0", MaxRetries: -1})
			client.AddHook(hook)
			t.Cleanup(func() { _ = client.Close() })
			s := NewStore(client, Options{})
			task := &v1alpha1.Task{
				Metadata: &v1alpha1.ObjectMeta{Name: "task"},
				Spec:     &v1alpha1.TaskSpec{Image: "alpine"},
			}

			err := s.SaveTask(context.Background(), task)
			if hook.reads != 2 {
				t.Fatalf("got %d reads, want 2", hook.reads)
			}
			if tc.secondErr == readErr {
				if !errors.Is(err, readErr) {
					t.Fatalf("got error %v, want %v", err, readErr)
				}
				if task.Status != nil {
					t.Fatalf("failed save changed caller status: %v", task.Status)
				}
				if len(hook.payloads) != 1 {
					t.Fatalf("got %d transactions after read failure, want 1", len(hook.payloads))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(hook.payloads) != 2 {
				t.Fatalf("got %d transactions, want 2", len(hook.payloads))
			}
			var saved v1alpha1.Task
			if err := protojson.Unmarshal(hook.payloads[1], &saved); err != nil {
				t.Fatal(err)
			}
			for _, status := range []*v1alpha1.TaskStatus{saved.Status, task.Status} {
				if status.GetPhase() != tc.phase || status.GetId() != tc.id {
					t.Fatalf("got status %v, want phase %q and id %q", status, tc.phase, tc.id)
				}
			}
		})
	}
}
