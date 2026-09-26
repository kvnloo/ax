package main

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

func TestParseGetArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    getTarget
		wantErr bool
	}{
		// Fork-main (pre-#383) precedence: a plural resource word lists even
		// when a name follows, mirroring the base condition
		// resource=="tasks" || (resource=="task" && len(args)==1). The
		// plural-with-name get form is Kevin's fix/get-plural-name-383 lane.
		{"plural task with name lists", []string{"tasks", "mytask"}, getTarget{kind: v1alpha1.KindTask}, false},
		{"plural gateway with name lists", []string{"gateways", "my-gw"}, getTarget{kind: v1alpha1.KindGateway}, false},
		{"plural workspace with name lists", []string{"workspaces", "my-ws"}, getTarget{kind: v1alpha1.KindWorkspace}, false},
		{"plural model with name lists", []string{"models", "my-model"}, getTarget{kind: v1alpha1.KindModel}, false},
		// Singular forms keep working.
		{"singular task with name", []string{"task", "mytask"}, getTarget{kind: v1alpha1.KindTask, name: "mytask"}, false},
		{"singular gateway with name", []string{"gateway", "my-gw"}, getTarget{kind: v1alpha1.KindGateway, name: "my-gw"}, false},
		{"singular workspace with name", []string{"workspace", "my-ws"}, getTarget{kind: v1alpha1.KindWorkspace, name: "my-ws"}, false},
		{"singular model with name", []string{"model", "my-model"}, getTarget{kind: v1alpha1.KindModel, name: "my-model"}, false},
		// Bare resource lists.
		{"plural task lists", []string{"tasks"}, getTarget{kind: v1alpha1.KindTask}, false},
		{"singular task lists", []string{"task"}, getTarget{kind: v1alpha1.KindTask}, false},
		{"plural gateway lists", []string{"gateways"}, getTarget{kind: v1alpha1.KindGateway}, false},
		{"plural workspace lists", []string{"workspaces"}, getTarget{kind: v1alpha1.KindWorkspace}, false},
		{"plural model lists", []string{"models"}, getTarget{kind: v1alpha1.KindModel}, false},
		// Case-insensitive like before.
		{"uppercase lists", []string{"TASKS"}, getTarget{kind: v1alpha1.KindTask}, false},
		{"mixed case plural with name lists", []string{"Gateways", "my-gw"}, getTarget{kind: v1alpha1.KindGateway}, false},
		// Errors.
		{"no args", []string{}, getTarget{}, true},
		{"unknown resource", []string{"pods"}, getTarget{}, true},
		{"unknown resource with name", []string{"pods", "x"}, getTarget{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseGetArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseGetArgs(%v) = %+v, want error", tc.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGetArgs(%v) error = %v", tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("parseGetArgs(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

// normalizeKind must accept singular/plural in any case. The old code ran
// TrimSuffix before ToLower, and TrimSuffix is case-sensitive, so
// "ax delete TASKS foo" errored while "ax delete tasks foo" worked.
func TestNormalizeKind(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"task", "Task", false},
		{"tasks", "Task", false},
		{"Task", "Task", false},
		{"TASKS", "Task", false},
		{"Tasks", "Task", false},
		{"gateway", "Gateway", false},
		{"gateways", "Gateway", false},
		{"GATEWAYS", "Gateway", false},
		{"Gateway", "Gateway", false},
		{"workspace", "Workspace", false},
		{"WORKSPACES", "Workspace", false},
		{"model", "Model", false},
		{"MODELS", "Model", false},
		{"Models", "Model", false},
		{"frobnicate", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := normalizeKind(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeKind(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeKind(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeKind(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// parseWatchArgs resolves "ax watch task <name>" to the task name. The old
// code ignored args[0] entirely, so "ax watch gateway mygw" silently watched
// a task named "mygw".
func TestParseWatchArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"task kind", []string{"task", "foo"}, "foo", false},
		{"tasks plural", []string{"tasks", "foo"}, "foo", false},
		{"uppercase kind", []string{"Task", "foo"}, "foo", false},
		{"unknown kind errors", []string{"gateway", "foo"}, "", true},
		{"typo kind errors", []string{"taskk", "foo"}, "", true},
		{"single arg errors", []string{"foo"}, "", true},
		{"no args errors", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWatchArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseWatchArgs(%v) = %q, want error", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseWatchArgs(%v) unexpected error: %v", tt.args, err)
			}
			if got != tt.want {
				t.Fatalf("parseWatchArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

// parseTaskNameArgs backs "ax suspend|resume [task] <name>". A bare name is
// used as-is; with two or more args the first must name the task kind
// (case-insensitive). The old inline code had no case-fold and no kind check,
// so "ax suspend Task foo" targeted a task literally named "Task" and
// "ax suspend gateway foo" targeted a task literally named "gateway".
func TestParseTaskNameArgs(t *testing.T) {
	usage := "ax suspend task <name>"
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"bare name", []string{"foo"}, "foo", false},
		{"task kind", []string{"task", "foo"}, "foo", false},
		{"tasks plural", []string{"tasks", "foo"}, "foo", false},
		{"uppercase kind", []string{"Task", "foo"}, "foo", false},
		{"allcaps kind", []string{"TASKS", "foo"}, "foo", false},
		{"unknown kind errors", []string{"gateway", "foo"}, "", true},
		{"typo kind errors", []string{"taskk", "foo"}, "", true},
		{"kind word as bare name", []string{"gateway"}, "gateway", false},
		{"no args errors", nil, "", true},
		{"empty errors", []string{}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTaskNameArgs(tt.args, usage)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseTaskNameArgs(%v) = %q, want error", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTaskNameArgs(%v) unexpected error: %v", tt.args, err)
			}
			if got != tt.want {
				t.Fatalf("parseTaskNameArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

// parseDescribeArgs must resolve singular/plural/case variants to the
// canonical kind, and must reject unknown kinds instead of silently
// describing a task (the old fallthrough behavior).
func TestParseDescribeArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantKind string
		wantName string
		wantErr  bool
	}{
		{"task singular", []string{"task", "foo"}, "Task", "foo", false},
		{"task plural", []string{"tasks", "foo"}, "Task", "foo", false},
		{"task uppercase", []string{"TASK", "foo"}, "Task", "foo", false},
		{"gateway singular", []string{"gateway", "gw1"}, "Gateway", "gw1", false},
		{"gateway plural", []string{"gateways", "gw1"}, "Gateway", "gw1", false},
		{"workspace singular", []string{"workspace", "ws1"}, "Workspace", "ws1", false},
		{"workspace plural", []string{"workspaces", "ws1"}, "Workspace", "ws1", false},
		{"model singular", []string{"model", "m1"}, "Model", "m1", false},
		{"model plural", []string{"models", "m1"}, "Model", "m1", false},
		{"unknown kind errors", []string{"frobnicate", "foo"}, "", "", true},
		{"typo kind errors", []string{"taskk", "foo"}, "", "", true},
		{"missing name", []string{"task"}, "", "", true},
		{"no args", nil, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDescribeArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseDescribeArgs(%v) = %+v, want error", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDescribeArgs(%v) unexpected error: %v", tt.args, err)
			}
			if got.kind != tt.wantKind || got.name != tt.wantName {
				t.Fatalf("parseDescribeArgs(%v) = %+v, want kind=%q name=%q", tt.args, got, tt.wantKind, tt.wantName)
			}
		})
	}
}

// TestKindNormalizationConsistent pins the consolidation: every command's
// argument parser must resolve the same kind spellings to the same canonical
// kind, because they all go through normalizeKind now.
func TestKindNormalizationConsistent(t *testing.T) {
	spellings := map[string]string{
		"task": v1alpha1.KindTask, "tasks": v1alpha1.KindTask,
		"Task": v1alpha1.KindTask, "TASKS": v1alpha1.KindTask,
		"gateway": v1alpha1.KindGateway, "GATEWAYS": v1alpha1.KindGateway,
		"workspace": v1alpha1.KindWorkspace, "Workspaces": v1alpha1.KindWorkspace,
		"model": v1alpha1.KindModel, "MODELS": v1alpha1.KindModel,
	}
	for spelling, want := range spellings {
		if got, err := normalizeKind(spelling); err != nil || got != want {
			t.Errorf("normalizeKind(%q) = %q, %v; want %q", spelling, got, err, want)
		}
		if tgt, err := parseGetArgs([]string{spelling}); err != nil || tgt.kind != want {
			t.Errorf("parseGetArgs(%q).kind = %q, %v; want %q", spelling, tgt.kind, err, want)
		}
		if tgt, err := parseDescribeArgs([]string{spelling, "x"}); err != nil || tgt.kind != want {
			t.Errorf("parseDescribeArgs(%q).kind = %q, %v; want %q", spelling, tgt.kind, err, want)
		}
	}
	// Task-only commands accept exactly the task spellings and reject the rest.
	for _, spelling := range []string{"task", "tasks", "Task", "TASKS"} {
		if _, err := parseWatchArgs([]string{spelling, "x"}); err != nil {
			t.Errorf("parseWatchArgs(%q) unexpected error: %v", spelling, err)
		}
		if _, err := parseTaskNameArgs([]string{spelling, "x"}, "usage"); err != nil {
			t.Errorf("parseTaskNameArgs(%q) unexpected error: %v", spelling, err)
		}
	}
	for _, spelling := range []string{"gateway", "GATEWAYS", "model", "workspace"} {
		if _, err := parseWatchArgs([]string{spelling, "x"}); err == nil {
			t.Errorf("parseWatchArgs(%q) = nil error, want error", spelling)
		}
		if _, err := parseTaskNameArgs([]string{spelling, "x"}, "usage"); err == nil {
			t.Errorf("parseTaskNameArgs(%q) = nil error, want error", spelling)
		}
	}
}
