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

package controller

import (
	"testing"
	"time"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// TestSetConditionTransitionTime pins the transition-timestamp contract:
// LastTransitionTime moves only when the condition's status actually changes.
// RED on base: setCondition bumped the timestamp on every call, so a steady
// "False/WorkspaceInitializing" condition reported a fresh transition time on
// every reconcile, making the timestamp meaningless.
func TestSetConditionTransitionTime(t *testing.T) {
	r := &TaskReconciler{}
	task := &v1alpha1.Task{Status: &v1alpha1.TaskStatus{}}

	t1 := time.Now().Truncate(time.Millisecond)
	r.setCondition(task, condReady, "False", "Initializing", "waiting", t1)
	if len(task.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(task.Status.Conditions))
	}

	// Same status, later clock, new reason/message: the timestamp must not move.
	t2 := t1.Add(time.Second)
	r.setCondition(task, condReady, "False", "StillInitializing", "still waiting", t2)
	got := task.Status.Conditions[0]
	if !got.LastTransitionTime.AsTime().Equal(t1) {
		t.Errorf("LastTransitionTime moved without a status transition: %v -> %v",
			t1, got.LastTransitionTime.AsTime())
	}
	if got.Reason != "StillInitializing" || got.Message != "still waiting" {
		t.Errorf("reason/message should still update: got %q/%q", got.Reason, got.Message)
	}

	// A genuine status transition moves the timestamp.
	t3 := t1.Add(2 * time.Second)
	r.setCondition(task, condReady, "True", "TaskRunning", "running", t3)
	got = task.Status.Conditions[0]
	if got.Status != "True" {
		t.Errorf("expected status True, got %q", got.Status)
	}
	if !got.LastTransitionTime.AsTime().Equal(t3) {
		t.Errorf("LastTransitionTime did not move on status transition: got %v, want %v",
			got.LastTransitionTime.AsTime(), t3)
	}
}

// TestWorkspaceReadyURL pins the readyz URL construction. RED on base: the
// URL was built with fmt.Sprintf("http://%s:%s/..."), so a bare IPv6 worker
// IP produced "http://fd00::1:80/readyz?check=workspace" — an undiallable
// URL — and workspace readiness was never detected on IPv6 clusters, leaving
// the task stuck Initializing. IPv4/hostname behavior is unchanged.
func TestWorkspaceReadyURL(t *testing.T) {
	for _, tc := range []struct{ workerIP, want string }{
		{"10.244.1.42", "http://10.244.1.42:80/readyz?check=workspace"},
		{"10.244.1.42:8080", "http://10.244.1.42:8080/readyz?check=workspace"},
		{"fd00::1", "http://[fd00::1]:80/readyz?check=workspace"},
		{"[fd00::1]:8080", "http://[fd00::1]:8080/readyz?check=workspace"},
	} {
		if got := workspaceReadyURL(tc.workerIP); got != tc.want {
			t.Errorf("workspaceReadyURL(%q) = %q, want %q", tc.workerIP, got, tc.want)
		}
	}
}
