package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

func TestEmptyListMessage(t *testing.T) {
	if got := emptyListMessage("tasks", "default"); got != `No tasks found in atespace "default".` {
		t.Errorf("emptyListMessage = %q", got)
	}
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns
// what fn printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// An empty atespace must say so instead of printing a bare table header:
// the header alone is ambiguous about whether the list even ran (ax's own
// tunnel list already prints its empty line; kubectl prints "No resources
// found").
func TestRunGetEmptyTasksPrintsMessage(t *testing.T) {
	fc := &fakeAXClient{} // ListTasks returns no tasks
	out := captureStdout(t, func() {
		if err := runGetWithClient(context.Background(), fc, "default", []string{"tasks"}); err != nil {
			t.Errorf("runGetWithClient: %v", err)
		}
	})
	if !strings.Contains(out, `No tasks found in atespace "default".`) {
		t.Errorf("missing empty-list message, got %q", out)
	}
	if strings.Contains(out, "NAME") {
		t.Errorf("table header printed for an empty list, got %q", out)
	}
}

// A non-empty list still renders the table, not the message.
func TestRunGetNonEmptyTasksPrintsTable(t *testing.T) {
	fc := &fakeAXClient{listedTasks: []*v1alpha1.Task{{}}}
	out := captureStdout(t, func() {
		if err := runGetWithClient(context.Background(), fc, "default", []string{"tasks"}); err != nil {
			t.Errorf("runGetWithClient: %v", err)
		}
	})
	if !strings.Contains(out, "NAME") {
		t.Errorf("table header missing for non-empty list, got %q", out)
	}
	if strings.Contains(out, "No tasks found") {
		t.Errorf("empty-list message printed for non-empty list, got %q", out)
	}
}
