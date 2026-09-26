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

// Validation happens before any RPC, so these tests need no server: the
// unreachable address only matters on the base commit, where the typo falls
// through to a real GetTask/WatchTask call.
const badServer = "http://127.0.0.1:1"

// TestDescribeUnknownKindRejected: a typo'd kind must fail fast with
// "unsupported kind", not silently describe a task of that name.
func TestDescribeUnknownKindRejected(t *testing.T) {
	err := runDescribe(badServer, "test", []string{"typo", "name"})
	if err == nil || !strings.Contains(err.Error(), `unsupported kind "typo"`) {
		t.Fatalf("runDescribe(typo) err = %v, want unsupported kind", err)
	}
}

// TestWatchUnknownKindRejected: ax watch ignored its kind argument entirely,
// so `ax watch nonsense name` watched task "name".
func TestWatchUnknownKindRejected(t *testing.T) {
	err := runWatch(badServer, "test", []string{"nonsense", "name"})
	if err == nil || !strings.Contains(err.Error(), `unsupported kind "nonsense"`) {
		t.Fatalf("runWatch(nonsense) err = %v, want unsupported kind", err)
	}
}

// TestWatchNonTaskKindRejected: watch only supports tasks; a valid but wrong
// kind must not fall through to watching a task.
func TestWatchNonTaskKindRejected(t *testing.T) {
	err := runWatch(badServer, "test", []string{"gateway", "name"})
	if err == nil || !strings.Contains(err.Error(), "ax watch task <name>") {
		t.Fatalf("runWatch(gateway) err = %v, want the watch usage error", err)
	}
}

// TestNormalizeKindPlurals: the shared helper accepts plural and mixed-case
// kinds, which describe/watch now route through.
func TestNormalizeKindPlurals(t *testing.T) {
	for _, k := range []string{"tasks", "Task", "GATEWAYS", "workspaces", "Models"} {
		if _, err := normalizeKind(k); err != nil {
			t.Fatalf("normalizeKind(%q): %v", k, err)
		}
	}
	if _, err := normalizeKind("typo"); err == nil {
		t.Fatal("normalizeKind(typo): want error")
	}
}
