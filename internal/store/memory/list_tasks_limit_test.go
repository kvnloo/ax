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

package memory

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// ListTasks with a non-positive limit must match the Redis backend, which
// substitutes the 50-row default. Returning the whole table here means the
// same CLI/server call sees a different page size per backend.
func TestListTasksNonPositiveLimitDefaultsToFifty(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	const n = 60
	for i := 0; i < n; i++ {
		if err := s.SaveTask(ctx, &v1alpha1.Task{
			Metadata: &v1alpha1.ObjectMeta{Name: fmt.Sprintf("t%02d", i)},
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
	}

	for _, limit := range []int64{0, -1} {
		got, err := s.ListTasks(ctx, "", limit, 0)
		if err != nil {
			t.Fatalf("ListTasks(limit=%d): %v", limit, err)
		}
		if len(got) != 50 {
			t.Fatalf("ListTasks(limit=%d) returned %d rows, want 50 (redis default)", limit, len(got))
		}
	}
}
