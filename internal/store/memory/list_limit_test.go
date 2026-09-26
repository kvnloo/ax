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
	"testing"
)

// TestListTasksLimitDefaultsToOnePage: the Redis backend maps a non-positive
// limit to its default page size (50); the memory store returned the whole
// set for limit<=0, and a negative offset panicked the server-side slice.
// Direct store callers must see identical paging semantics on both backends.
func TestListTasksLimitDefaultsToOnePage(t *testing.T) {
	s := NewStore()
	saveTaskNames(t, s, 120)

	if got := listNames(t, s, 0, 0); len(got) != 50 {
		t.Fatalf("ListTasks(limit=0) = %d tasks, want 50 (Redis default page)", len(got))
	}
	if got := listNames(t, s, -10, 50); len(got) != 50 {
		t.Fatalf("ListTasks(limit=-10, offset=50) = %d tasks, want 50", len(got))
	}
	// A negative offset must not panic; it is clamped to the head of the list.
	if got := listNames(t, s, 10, -1); len(got) != 10 {
		t.Fatalf("ListTasks(limit=10, offset=-1) = %d tasks, want 10", len(got))
	}
}
