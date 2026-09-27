package memory_test

import (
	"context"
	"testing"

	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/pkg/apis/v1alpha1"
)

func seedTasks(t *testing.T, s *memory.MemoryStore, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		err := s.SaveTask(context.Background(), &v1alpha1.Task{
			Metadata: &v1alpha1.ObjectMeta{Name: string(rune('a' + i))},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// A negative offset must behave as the head of the list (Redis parity), not panic.
func TestListTasksNegativeOffsetClampedToHead(t *testing.T) {
	s := memory.NewStore()
	seedTasks(t, s, 3)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ListTasks(negative offset) panicked: %v", r)
		}
	}()
	got, err := s.ListTasks(context.Background(), "", 50, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d tasks, want 3 (clamped to head)", len(got))
	}
	head, err := s.ListTasks(context.Background(), "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(head) != 3 {
		t.Fatalf("got %d tasks at offset 0, want 3", len(head))
	}
}
