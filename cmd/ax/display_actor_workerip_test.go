package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// TestDisplayActor: whitespace-only counts as unset and reads "<none>",
// matching the `ax get tasks` ACTOR column rule the helpers consolidate.
func TestDisplayActor(t *testing.T) {
	for in, want := range map[string]string{
		"":         "<none>",
		"   ":      "<none>",
		"\t\n":     "<none>",
		"runner":   "runner",
		" runner ": "runner",
	} {
		if got := displayActor(in); got != want {
			t.Errorf("displayActor(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDisplayWorkerIP: same rule for the WORKER-IP column.
func TestDisplayWorkerIP(t *testing.T) {
	for in, want := range map[string]string{
		"":           "<none>",
		"   ":        "<none>",
		"10.0.0.4":   "10.0.0.4",
		" 10.0.0.4 ": "10.0.0.4",
	} {
		if got := displayWorkerIP(in); got != want {
			t.Errorf("displayWorkerIP(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestWatchStreamLoopDisplayRules: the `ax watch` event line used to print
// the raw phase/actor/workerIP, so a status-less task rendered a blank
// Phase (get-list/describe say "Pending") and blank Actor/WorkerIP
// (get-list says "<none>"). The banner now goes through the shared display
// helpers. Red on base: output contains "Phase:            " with nothing
// after it and no "<none>".
func TestWatchStreamLoopDisplayRules(t *testing.T) {
	stream := &stubWatchStream{resps: []*v1alpha1.WatchTaskResponse{
		{Task: &v1alpha1.Task{}},
		{Task: &v1alpha1.Task{Status: &v1alpha1.TaskStatus{Phase: "Completed"}}},
	}}
	var buf bytes.Buffer
	if err := watchStreamLoop(stream, &buf, "default", "t"); err != nil {
		t.Fatalf("watchStreamLoop err = %v, want nil", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Pending") {
		t.Errorf("event line missing displayPhase rule; output:\n%s", out)
	}
	if strings.Count(out, "<none>") < 2 {
		t.Errorf("event line missing <none> actor/workerIP rule; output:\n%s", out)
	}
}
