package main

import (
	"context"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// The describe detail views rendered object names, atespaces, and the
// conditions table raw: a name like "evil\nname" (persistable — ValidateTask
// only validates spec.workspaceRefs, applyDocument only rejects
// whitespace-only names) printed fake extra lines into the output, and a
// tab in an atespace broke the tabwriter alignment of the Conditions
// section. The get-list views were fixed in the previous wave; describe is
// the same defect class on a new surface.
func TestRunDescribeTaskSanitizesControlChars(t *testing.T) {
	fc := &fakeAXClient{getTask: &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "evil\nname", Atespace: "def\ttault"},
		Status: &v1alpha1.TaskStatus{
			Phase: "Running",
			Conditions: []*v1alpha1.Condition{
				{Type: "Ready", Status: "False", Reason: "Reconciling", Message: "pulling im\ng"},
			},
		},
	}}
	out := captureStdout(t, func() {
		if err := runDescribeWithClient(context.Background(), fc, "default", v1alpha1.KindTask, "evil\nname"); err != nil {
			t.Errorf("runDescribeWithClient: %v", err)
		}
	})
	// The injected newline in the name must not produce extra output lines.
	if strings.Contains(out, "evil\nname") {
		t.Errorf("describe output contains a raw newline from the task name: %q", out)
	}
	if !strings.Contains(out, "Name:         evil name") {
		t.Errorf("expected the sanitized name line, got: %q", out)
	}
	// The tab in the atespace and the newline in the condition message must
	// not survive either.
	if strings.Contains(out, "def\ttault") || strings.Contains(out, "pulling im\ng") {
		t.Errorf("describe output contains raw control characters: %q", out)
	}
	if !strings.Contains(out, "Atespace:     def tault") {
		t.Errorf("expected the sanitized atespace line, got: %q", out)
	}
	// The Conditions section must stay a single header row + one data row.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	cond := 0
	for _, l := range lines {
		if strings.Contains(l, "Ready") {
			cond++
		}
	}
	if cond != 1 {
		t.Errorf("expected exactly 1 condition row, got %d in %q", cond, out)
	}
}
