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

func TestFormatApplySummaryMixed(t *testing.T) {
	results := []applyResult{
		{kind: "Task", name: "a", outcome: "created"},
		{kind: "Gateway", name: "b", outcome: "unchanged"},
		{kind: "Task", name: "c", outcome: "configured"},
		{kind: "Task", name: "d", outcome: "created"},
	}
	got := formatApplySummary(results)
	want := "applied 4 resources: 2 created, 1 configured, 1 unchanged"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatApplySummarySkipsZeroCounts(t *testing.T) {
	got := formatApplySummary([]applyResult{
		{kind: "Task", name: "a", outcome: "unchanged"},
		{kind: "Task", name: "b", outcome: "unchanged"},
	})
	want := "applied 2 resources: 2 unchanged"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatApplySummarySingular(t *testing.T) {
	got := formatApplySummary([]applyResult{
		{kind: "Task", name: "a", outcome: "created"},
	})
	want := "applied 1 resource: 1 created"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatApplySummaryOrderIndependent(t *testing.T) {
	a := formatApplySummary([]applyResult{
		{kind: "Task", name: "a", outcome: "unchanged"},
		{kind: "Task", name: "b", outcome: "created"},
	})
	b := formatApplySummary([]applyResult{
		{kind: "Task", name: "b", outcome: "created"},
		{kind: "Task", name: "a", outcome: "unchanged"},
	})
	if a != b {
		t.Fatalf("summary order depends on input order: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "applied 2 resources: ") {
		t.Fatalf("unexpected summary shape: %q", a)
	}
}
