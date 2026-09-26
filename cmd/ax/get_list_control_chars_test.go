package main

import (
	"context"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// Object names reach the server without control-character validation
// (ValidateTask only validates spec.workspaceRefs; applyDocument only
// rejects whitespace-only names). A name like "evil\nname" or "a\tb" is
// persisted, and `ax get` renders NAME/ATESPACE raw into the tabwriter:
// the newline splits the row, the tab shifts the columns. The table must
// stay one row per object.
func TestRunGetTasksSanitizesControlCharsInNames(t *testing.T) {
	fc := &fakeAXClient{listedTasks: []*v1alpha1.Task{
		{Metadata: &v1alpha1.ObjectMeta{Name: "evil\nname", Atespace: "def\tault"},
			Status: &v1alpha1.TaskStatus{Phase: "Running"}},
		{Metadata: &v1alpha1.ObjectMeta{Name: "plain", Atespace: "default"},
			Status: &v1alpha1.TaskStatus{Phase: "Running"}},
	}}
	out := captureStdout(t, func() {
		if err := runGetWithClient(context.Background(), fc, "default", []string{"tasks"}); err != nil {
			t.Errorf("runGetWithClient: %v", err)
		}
	})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// header + 2 rows; a raw newline in the name would make 4 lines.
	if len(lines) != 3 {
		t.Fatalf("expected 3 output lines (header + 2 rows), got %d: %q", len(lines), out)
	}
	// No tabwriter control characters may survive in the body rows.
	for _, l := range lines[1:] {
		if strings.ContainsAny(l, "\t") {
			t.Errorf("row contains a raw tab: %q", l)
		}
	}
}
