// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on a "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package redis

import (
	"testing"

	"github.com/redis/go-redis/v9"
)

// Task events are published on every save and every delete mark, and stream
// entries are never removed by the consumers (XACK only clears the pending
// list). Without a cap the stream grows without bound and Redis memory with
// it. The memory store's event channel is already bounded (1000); the Redis
// stream must be capped too.
func TestStreamEventArgsCapped(t *testing.T) {
	args := streamEventArgs("ax:stream:tasks", "reconcile", "default", "my-task")
	if args.MaxLen <= 0 {
		t.Errorf("stream XAdd MaxLen = %d; want a positive cap", args.MaxLen)
	}
	if !args.Approx {
		t.Errorf("stream XAdd Approx = false; want approximate trimming for O(1) amortized cost")
	}
	if args.Stream != "ax:stream:tasks" {
		t.Errorf("stream = %q; want %q", args.Stream, "ax:stream:tasks")
	}
	want := map[string]string{"action": "reconcile", "atespace": "default", "name": "my-task"}
	vals, ok := args.Values.(map[string]interface{})
	if !ok {
		t.Fatalf("values has type %T; want map[string]interface{}", args.Values)
	}
	for k, v := range want {
		got, ok := vals[k].(string)
		if !ok || got != v {
			t.Errorf("values[%q] = %v; want %q", k, vals[k], v)
		}
	}
	var _ *redis.XAddArgs = args
}
