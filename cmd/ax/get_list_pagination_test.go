package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
)

// The server caps ListTasks at 50 rows per page (internal/server ListTasks:
// limit defaults to 50 when the request asks for none). `ax get tasks`
// previously issued a single limit-less request, so a fleet with more than
// 50 tasks silently showed only the first page with no truncation notice.
// The CLI must page through the whole set.
type paginatingTaskClient struct {
	fakeAXClient
	tasks []*v1alpha1.Task
	calls int
}

func (p *paginatingTaskClient) ListTasks(ctx context.Context, in *v1alpha1.ListTasksRequest, opts ...grpc.CallOption) (*v1alpha1.ListTasksResponse, error) {
	p.calls++
	limit := in.GetLimit()
	if limit <= 0 {
		limit = 50 // server default, mirrored from internal/server
	}
	start := int(in.GetOffset())
	if start > len(p.tasks) {
		start = len(p.tasks)
	}
	end := start + int(limit)
	if end > len(p.tasks) {
		end = len(p.tasks)
	}
	return &v1alpha1.ListTasksResponse{Tasks: p.tasks[start:end]}, nil
}

func TestRunGetTasksPagesPastServerLimit(t *testing.T) {
	var tasks []*v1alpha1.Task
	for i := 0; i < 120; i++ {
		tasks = append(tasks, &v1alpha1.Task{
			Metadata: &v1alpha1.ObjectMeta{Name: fmt.Sprintf("task-%03d", i), Atespace: "default"},
			Status:   &v1alpha1.TaskStatus{Phase: "Running"},
		})
	}
	fc := &paginatingTaskClient{tasks: tasks}
	out := captureStdout(t, func() {
		if err := runGetWithClient(context.Background(), fc, "default", []string{"tasks"}); err != nil {
			t.Errorf("runGetWithClient: %v", err)
		}
	})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// header + 120 rows; a single uncapped request returns only 50.
	if len(lines) != 121 {
		t.Fatalf("expected 121 output lines (header + 120 rows), got %d", len(lines))
	}
	if fc.calls < 2 {
		t.Errorf("expected multiple ListTasks pages, got %d call(s)", fc.calls)
	}
}
