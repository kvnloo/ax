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
	"context"
	"testing"
	"time"

	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/pkg/apis/v1alpha1"
)

func requeueTestTask(name, phase, workerIP string, workspaceReady bool) *v1alpha1.Task {
	readyStatus := "False"
	if workspaceReady {
		readyStatus = "True"
	}
	return &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: name, Atespace: "default"},
		Spec:     &v1alpha1.TaskSpec{},
		Status: &v1alpha1.TaskStatus{
			Phase:    phase,
			WorkerIp: workerIP,
			Conditions: []*v1alpha1.Condition{
				{Type: condWorkspaceReady, Status: readyStatus, Reason: "Initializing"},
			},
		},
	}
}

func TestReadinessRequeueNeeded(t *testing.T) {
	w := NewWorker(memory.NewStore(), NewTaskReconciler(nil, "t", "a"), "g", "c")
	cases := []struct {
		name string
		task *v1alpha1.Task
		want bool
	}{
		{"running initializing", requeueTestTask("a", "Running", "10.0.0.1", false), true},
		{"running ready", requeueTestTask("b", "Running", "10.0.0.1", true), false},
		{"pending no worker", requeueTestTask("c", "Pending", "", false), true},
		{"pending with worker", requeueTestTask("d", "Pending", "10.0.0.1", false), false},
		{"suspended", requeueTestTask("e", "Suspended", "10.0.0.1", false), false},
		{"failed", requeueTestTask("f", "Failed", "10.0.0.1", false), false},
		{"terminating", requeueTestTask("g", "Terminating", "10.0.0.1", false), false},
		{"nil task", nil, false},
		{"nil status", &v1alpha1.Task{}, false},
	}
	for _, tc := range cases {
		if got := w.readinessRequeueNeeded(tc.task); got != tc.want {
			t.Errorf("%s: readinessRequeueNeeded = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestScheduleRequeueRepublishesEvent is the behavioral red test: a task left
// initializing must get a fresh reconcile event after the requeue delay. On
// base (no scheduleRequeue) this file does not compile.
func TestScheduleRequeueRepublishesEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := memory.NewStore()
	sub, err := st.Subscribe(ctx, "g", "c")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := st.SaveTask(ctx, requeueTestTask("rq", "Running", "10.0.0.1", false)); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := sub.Next(ctx); err != nil { // drain the seed event
		t.Fatalf("drain seed event: %v", err)
	}

	w := NewWorker(st, NewTaskReconciler(nil, "t", "a"), "g", "c")
	w.requeueDelay = 20 * time.Millisecond
	w.scheduleRequeue(ctx, "default", "rq")

	rctx, rcancel := context.WithTimeout(ctx, 3*time.Second)
	defer rcancel()
	ev, err := sub.Next(rctx)
	if err != nil {
		t.Fatalf("no republished reconcile event arrived: %v", err)
	}
	if ev.Action != "reconcile" || ev.Name != "rq" || ev.Atespace != "default" {
		t.Errorf("unexpected republished event: %+v", ev)
	}
}

func TestScheduleRequeueSkipsResolvedTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := memory.NewStore()
	sub, err := st.Subscribe(ctx, "g", "c")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := st.SaveTask(ctx, requeueTestTask("ok", "Running", "10.0.0.1", true)); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := sub.Next(ctx); err != nil {
		t.Fatalf("drain seed event: %v", err)
	}

	w := NewWorker(st, NewTaskReconciler(nil, "t", "a"), "g", "c")
	w.requeueDelay = 20 * time.Millisecond
	w.scheduleRequeue(ctx, "default", "ok")

	rctx, rcancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer rcancel()
	if ev, err := sub.Next(rctx); err == nil {
		t.Errorf("resolved task was requeued: %+v", ev)
	}
}

func TestScheduleRequeueSkipsDeletedTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := memory.NewStore()
	sub, err := st.Subscribe(ctx, "g", "c")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := st.SaveTask(ctx, requeueTestTask("gone", "Running", "10.0.0.1", false)); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := sub.Next(ctx); err != nil {
		t.Fatalf("drain seed event: %v", err)
	}
	if err := st.DeleteTask(ctx, "default", "gone"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	w := NewWorker(st, NewTaskReconciler(nil, "t", "a"), "g", "c")
	w.requeueDelay = 20 * time.Millisecond
	w.scheduleRequeue(ctx, "default", "gone")

	rctx, rcancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer rcancel()
	if ev, err := sub.Next(rctx); err == nil {
		t.Errorf("deleted task was resurrected by requeue: %+v", ev)
	}
}
