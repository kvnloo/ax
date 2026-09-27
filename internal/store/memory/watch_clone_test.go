package memory

import (
	"context"
	"testing"
	"time"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// A watcher must receive its own copy of the task: mutating the watched task
// must not rewrite the store's internal record.
func TestWatchTaskHandsOutPrivateCopies(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	mk := func() *v1alpha1.Task {
		return &v1alpha1.Task{
			Metadata: &v1alpha1.ObjectMeta{Name: "t1", Atespace: "default"},
			Status:   &v1alpha1.TaskStatus{Phase: "Running"},
		}
	}
	if err := s.SaveTask(ctx, mk()); err != nil {
		t.Fatal(err)
	}
	ch, closer, err := s.WatchTask(ctx, "default", "t1")
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	// A second save notifies the watcher.
	if err := s.SaveTask(ctx, mk()); err != nil {
		t.Fatal(err)
	}

	var watched *v1alpha1.Task
	select {
	case watched = <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("no watch event delivered")
	}
	// A hostile watcher mutates everything it can reach.
	watched.Status.Phase = "CorruptedByWatcher"
	watched.Metadata.Name = "renamed"
	watched.Status.Conditions = nil

	back, err := s.GetTask(ctx, "default", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if back.Status.Phase != "Running" {
		t.Errorf("stored phase rewritten by watcher: got %q, want %q", back.Status.Phase, "Running")
	}
	if back.Metadata.Name != "t1" {
		t.Errorf("stored name rewritten by watcher: got %q, want %q", back.Metadata.Name, "t1")
	}
}

// Two watchers on the same task must not share a task object either.
func TestWatchTaskWatchersDoNotShareCopies(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	task := &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "t1", Atespace: "default"},
		Status:   &v1alpha1.TaskStatus{Phase: "Running"},
	}
	if err := s.SaveTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	ch1, c1, err := s.WatchTask(ctx, "default", "t1")
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	ch2, c2, err := s.WatchTask(ctx, "default", "t1")
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	if err := s.SaveTask(ctx, &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "t1", Atespace: "default"},
		Status:   &v1alpha1.TaskStatus{Phase: "Running"},
	}); err != nil {
		t.Fatal(err)
	}

	recv := func(ch <-chan *v1alpha1.Task) *v1alpha1.Task {
		t.Helper()
		select {
		case got := <-ch:
			return got
		case <-time.After(2 * time.Second):
			t.Fatal("no watch event delivered")
			return nil
		}
	}
	got1, got2 := recv(ch1), recv(ch2)
	if got1 == got2 {
		t.Fatal("both watchers received the same *Task pointer")
	}
	got1.Status.Phase = "CorruptedByWatcher"
	if got2.Status.Phase != "Running" {
		t.Errorf("watcher 1 mutation visible to watcher 2: got %q", got2.Status.Phase)
	}
}
