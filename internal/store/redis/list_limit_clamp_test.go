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
	"math"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/ax/pkg/apis/v1alpha1"
	goredis "github.com/redis/go-redis/v9"
)

// A client-controlled int64 limit combined with a nonzero offset overflowed
// offset+limit-1 into a negative ZREVRANGE stop, which Redis normalizes to an
// empty page even though tasks remain. The stop must saturate instead.
func TestListTasksHugeLimitOffsetSaturates(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	s := NewStore(goredis.NewClient(&goredis.Options{Addr: mr.Addr()}), Options{})
	ctx := context.Background()
	for _, n := range []string{"a", "b", "c"} {
		if err := s.SaveTask(ctx, &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: n}}); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := s.ListTasks(ctx, "", math.MaxInt64, 1)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks (offset 1 of 3), got %d", len(tasks))
	}
}
