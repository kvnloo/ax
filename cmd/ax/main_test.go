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

package main

import (
	"strings"
	"testing"
)

func TestRejectExtraArgs(t *testing.T) {
	if err := rejectExtraArgs(nil); err != nil {
		t.Fatalf("no args: unexpected error %v", err)
	}
	if err := rejectExtraArgs([]string{}); err != nil {
		t.Fatalf("empty args: unexpected error %v", err)
	}
	err := rejectExtraArgs([]string{"prod"})
	if err == nil {
		t.Fatalf("extra arg: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected argument") || !strings.Contains(err.Error(), `"prod"`) {
		t.Fatalf("extra arg: unexpected message %q", err)
	}
}
