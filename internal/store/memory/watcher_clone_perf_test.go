package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// realisticTask builds a task shaped like a mid-life reconciled task: env vars,
// workspace refs, and the conditions Reconcile accumulates (Ready, WorkspaceReady,
// GatewayReady). Clone cost scales with message size, so the fixture must not be
// a degenerate empty task.
func realisticTask(name string) *v1alpha1.Task {
	env := make([]*v1alpha1.EnvVar, 0, 8)
	for i := 0; i < 8; i++ {
		env = append(env, &v1alpha1.EnvVar{Name: fmt.Sprintf("ENV_VAR_%d", i), Value: fmt.Sprintf("value-%d-with-some-length", i)})
	}
	refs := []*v1alpha1.WorkspaceRef{
		{Name: "ws-src", Path: "/workspace/src"},
		{Name: "ws-data", Path: "/workspace/data"},
		{Name: "ws-cache", Path: "/workspace/cache"},
	}
	now := timestamppb.Now()
	conds := []*v1alpha1.Condition{
		{Type: "Ready", Status: "True", Reason: "TaskRunning", Message: "Task is running and its workspace is ready", LastTransitionTime: now},
		{Type: "WorkspaceReady", Status: "True", Reason: "SetupComplete", Message: "Workspace setup completed at 10.0.0.5:80", LastTransitionTime: now},
		{Type: "GatewayReady", Status: "True", Reason: "PoliciesApplied", Message: "Network policies active", LastTransitionTime: now},
	}
	return &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: name, Atespace: "default"},
		Spec: &v1alpha1.TaskSpec{
			Image:      "us-docker.pkg.dev/ax/task-runner:latest",
			Env:        env,
			Workspaces: refs,
			Command:    []string{"/bin/bash", "-c", "make test"},
		},
		Status: &v1alpha1.TaskStatus{
			Phase:      "Running",
			Id:         "task-" + name + "-1720000000",
			Actor:      name,
			WorkerIp:   "10.0.0.5:80",
			Conditions: conds,
		},
	}
}

// UpdateTaskStatus and MarkTaskDeleting used to proto.Clone the whole task on
// every call even when no watcher was registered for it. That clone exists only
// for watcher delivery; the common no-watcher case (every worker reconcile)
// paid for it under the write lock for nothing.

// TestUpdateTaskStatusNoWatchersUpdatesStored guards the fast path: with no
// watchers the status must still land in the store.
func TestUpdateTaskStatusNoWatchersUpdatesStored(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	if err := s.SaveTask(ctx, realisticTask("t1")); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateTaskStatus(ctx, "default", "t1", &v1alpha1.TaskStatus{Phase: "Suspended"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTask(ctx, "default", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != "Suspended" {
		t.Fatalf("stored phase = %q, want Suspended", got.Status.Phase)
	}
}

// TestUpdateTaskStatusStillNotifiesWatcher guards the slow path: a registered
// watcher must still receive the updated task.
func TestUpdateTaskStatusStillNotifiesWatcher(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	if err := s.SaveTask(ctx, realisticTask("t2")); err != nil {
		t.Fatal(err)
	}
	ch, closer, err := s.WatchTask(ctx, "default", "t2")
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	if err := s.UpdateTaskStatus(ctx, "default", "t2", &v1alpha1.TaskStatus{Phase: "Failed"}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-ch:
		if got.Status.Phase != "Failed" {
			t.Fatalf("watcher got phase %q, want Failed", got.Status.Phase)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watcher received nothing after UpdateTaskStatus")
	}
}

// TestMarkTaskDeletingNoWatchersMarksTerminating guards the fast path for deletes.
func TestMarkTaskDeletingNoWatchersMarksTerminating(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	if err := s.SaveTask(ctx, realisticTask("t3")); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkTaskDeleting(ctx, "default", "t3"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTask(ctx, "default", "t3")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != v1alpha1.PhaseTerminating {
		t.Fatalf("stored phase = %q, want %q", got.Status.Phase, v1alpha1.PhaseTerminating)
	}
}

// TestMarkTaskDeletingStillNotifiesWatcher guards the slow path for deletes.
func TestMarkTaskDeletingStillNotifiesWatcher(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	if err := s.SaveTask(ctx, realisticTask("t4")); err != nil {
		t.Fatal(err)
	}
	ch, closer, err := s.WatchTask(ctx, "default", "t4")
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	if err := s.MarkTaskDeleting(ctx, "default", "t4"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-ch:
		if got.Status.Phase != v1alpha1.PhaseTerminating {
			t.Fatalf("watcher got phase %q, want %q", got.Status.Phase, v1alpha1.PhaseTerminating)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watcher received nothing after MarkTaskDeleting")
	}
}

func BenchmarkUpdateTaskStatusNoWatchers(b *testing.B) {
	s := NewStore()
	ctx := context.Background()
	if err := s.SaveTask(ctx, realisticTask("bench-task")); err != nil {
		b.Fatal(err)
	}
	status := realisticTask("bench-task").Status
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.UpdateTaskStatus(ctx, "default", "bench-task", status); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarkTaskDeletingNoWatchers(b *testing.B) {
	s := NewStore()
	ctx := context.Background()
	if err := s.SaveTask(ctx, realisticTask("bench-del")); err != nil {
		b.Fatal(err)
	}
	// Drain the event channel so every iteration's publish behaves identically
	// (otherwise the first 1000 iterations send and the rest drop).
	go func() {
		for range s.events {
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.MarkTaskDeleting(ctx, "default", "bench-del"); err != nil {
			b.Fatal(err)
		}
	}
}
