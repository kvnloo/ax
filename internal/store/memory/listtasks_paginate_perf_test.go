package memory

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// realisticListTask is a mid-size task for list benchmarks: the old ListTasks
// cloned every match before slicing, so per-task size matters.
func realisticListTask(name, atespace string) *v1alpha1.Task {
	env := make([]*v1alpha1.EnvVar, 0, 8)
	for i := 0; i < 8; i++ {
		env = append(env, &v1alpha1.EnvVar{Name: fmt.Sprintf("ENV_VAR_%d", i), Value: fmt.Sprintf("value-%d-with-some-length", i)})
	}
	now := timestamppb.Now()
	return &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: name, Atespace: atespace},
		Spec: &v1alpha1.TaskSpec{
			Image:   "us-docker.pkg.dev/ax/task-runner:latest",
			Env:     env,
			Command: []string{"/bin/bash", "-c", "make test"},
		},
		Status: &v1alpha1.TaskStatus{
			Phase: "Running",
			Conditions: []*v1alpha1.Condition{
				{Type: "Ready", Status: "True", Reason: "TaskRunning", Message: "running", LastTransitionTime: now},
			},
		},
	}
}

// ListTasks used to proto.Clone every matching task and only then apply
// offset/limit: O(N) clones for an O(limit) page. It now collects matches,
// slices, and clones only the returned page. Same elements, same order.

// TestListTasksPaginateCoversFullList: every page must have the right length,
// contain only tasks from the queried atespace, and carry no duplicates within
// itself. (Map iteration order is random per call on fork main, so cross-call
// page stability is not asserted here — that is a separate concern.)
func TestListTasksPaginateCoversFullList(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	const n = 37
	for i := 0; i < n; i++ {
		as := "default"
		if i%3 == 0 {
			as = "other"
		}
		if err := s.SaveTask(ctx, realisticListTask(fmt.Sprintf("t-%02d", i), as)); err != nil {
			t.Fatal(err)
		}
	}

	check := func(atespace string, total int) {
		t.Helper()
		full, err := s.ListTasks(ctx, atespace, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(full) != total {
			t.Fatalf("atespace %q: full list = %d tasks, want %d", atespace, len(full), total)
		}
		seen := map[string]bool{}
		for _, task := range full {
			seen[task.Metadata.Name] = true
		}
		if len(seen) != total {
			t.Fatalf("atespace %q: full list has duplicate names", atespace)
		}
		// One page per offset; each page must be well-formed on its own.
		for off := 0; off < total; off += 7 {
			page, err := s.ListTasks(ctx, atespace, 7, int64(off))
			if err != nil {
				t.Fatal(err)
			}
			wantLen := 7
			if total-off < 7 {
				wantLen = total - off
			}
			if len(page) != wantLen {
				t.Fatalf("atespace %q offset %d: page len = %d, want %d", atespace, off, len(page), wantLen)
			}
			inPage := map[string]bool{}
			for _, task := range page {
				if !seen[task.Metadata.Name] {
					t.Fatalf("atespace %q: page returned unknown task %q", atespace, task.Metadata.Name)
				}
				if inPage[task.Metadata.Name] {
					t.Fatalf("atespace %q: task %q duplicated within one page", atespace, task.Metadata.Name)
				}
				inPage[task.Metadata.Name] = true
			}
		}
	}
	check("default", 24) // i%3 != 0
	check("other", 13)   // i%3 == 0, i in [0,37)
	check("*", n)
	check("", n)
}

// TestListTasksPageReturnsClones: paged results must be private copies, exactly
// as the old clone-everything path produced.
func TestListTasksPageReturnsClones(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := s.SaveTask(ctx, realisticListTask(fmt.Sprintf("c-%d", i), "default")); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.ListTasks(ctx, "default", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 {
		t.Fatalf("page len = %d, want 2", len(page))
	}
	page[0].Status.Phase = "MUTATED"
	fresh, err := s.GetTask(ctx, "default", page[0].Metadata.Name)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status.Phase == "MUTATED" {
		t.Fatal("paged task aliases stored state: mutation leaked into the store")
	}
}

// TestListTasksPaginateEdges pins the boundary semantics the old code had:
// offset past the end yields empty, limit<=0 yields everything.
func TestListTasksPaginateEdges(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := s.SaveTask(ctx, realisticListTask(fmt.Sprintf("e-%d", i), "default")); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := s.ListTasks(ctx, "default", 10, 99); len(got) != 0 {
		t.Fatalf("offset past end = %d tasks, want 0", len(got))
	}
	if got, _ := s.ListTasks(ctx, "default", 0, 0); len(got) != 3 {
		t.Fatalf("limit 0 = %d tasks, want 3", len(got))
	}
	if got, _ := s.ListTasks(ctx, "default", 10, 2); len(got) != 1 {
		t.Fatalf("offset 2 limit 10 = %d tasks, want 1", len(got))
	}
}

func BenchmarkListTasksPaginated(b *testing.B) {
	s := NewStore()
	ctx := context.Background()
	for i := 0; i < 2000; i++ {
		if err := s.SaveTask(ctx, realisticListTask(fmt.Sprintf("bench-list-%d", i), "default")); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tasks, err := s.ListTasks(ctx, "default", 10, 0)
		if err != nil {
			b.Fatal(err)
		}
		if len(tasks) != 10 {
			b.Fatalf("got %d tasks, want 10", len(tasks))
		}
	}
}
