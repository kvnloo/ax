package main

import (
	"context"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// The actor/workerIP "<none>" fallback only checked == "": a whitespace-only
// value sailed through and rendered as a blank column. A blank cell reads as
// missing data; it must render like the empty case.
func TestRunGetTasksWhitespaceOnlyActorAndWorkerIP(t *testing.T) {
	fc := &fakeAXClient{listedTasks: []*v1alpha1.Task{
		{Metadata: &v1alpha1.ObjectMeta{Name: "ws-task", Atespace: "default"},
			Status: &v1alpha1.TaskStatus{Phase: "Running", Actor: "  ", WorkerIp: "\t"}},
	}}
	out := captureStdout(t, func() {
		if err := runGetWithClient(context.Background(), fc, "default", []string{"tasks"}); err != nil {
			t.Errorf("runGetWithClient: %v", err)
		}
	})
	row := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "ws-task") {
			row = line
		}
	}
	if row == "" {
		t.Fatalf("ws-task row missing, got %q", out)
	}
	if n := strings.Count(row, "<none>"); n != 2 {
		t.Errorf("expected ACTOR and WORKER-IP to render <none>, got %q", row)
	}
}

// A whitespace-only listener protocol rendered as "80/ " in `ax get
// gateways` (and "80 ( )" in describe). It must read like the empty case.
func TestListenerProtocolOmitsWhitespaceOnly(t *testing.T) {
	for _, p := range []string{" ", "  ", "\t"} {
		if got := listenerProtocol(&v1alpha1.Listener{Name: "web", Port: 80, Protocol: p}); got != "" {
			t.Errorf("listenerProtocol(%q) = %q, want \"\"", p, got)
		}
	}
}
