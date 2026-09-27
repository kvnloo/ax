package main

import "testing"

// parseTaskNameArgs must fix the old inline handling in runSuspend/runResume:
//   - no case-fold: `ax suspend Task foo` targeted a task literally named
//     "Task" (dropping "foo");
//   - no kind validation: `ax suspend gateway foo` targeted a task named
//     "gateway" instead of erroring;
//   - `ax suspend task foo bar` silently dropped "bar".
//
// A lone kind word as the single arg stays a legitimate bare name.
func TestParseTaskNameArgs(t *testing.T) {
	cases := []struct {
		name    string
		cmd     string
		args    []string
		wantErr bool
		want    string
	}{
		{"bare name", "suspend", []string{"foo"}, false, "foo"},
		{"lone kind word is a bare name", "suspend", []string{"task"}, false, "task"},
		{"kind + name", "suspend", []string{"task", "foo"}, false, "foo"},
		{"plural kind + name", "resume", []string{"tasks", "foo"}, false, "foo"},
		{"case-insensitive kind", "suspend", []string{"TASKS", "foo"}, false, "foo"},
		{"capitalized kind is not the name", "suspend", []string{"Task", "foo"}, false, "foo"},
		{"non-task kind is an error, not the name",
			"suspend", []string{"gateway", "foo"}, true, ""},
		{"extra args are an error", "suspend", []string{"task", "foo", "bar"}, true, ""},
		{"no args", "resume", nil, true, ""},
		{"unknown kind", "suspend", []string{"frobnicate", "x"}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTaskNameArgs(tc.cmd, tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseTaskNameArgs(%q, %v) = %q, nil; want error", tc.cmd, tc.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTaskNameArgs(%q, %v) error = %v", tc.cmd, tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("parseTaskNameArgs(%q, %v) = %q; want %q", tc.cmd, tc.args, got, tc.want)
			}
		})
	}
}

// normalizeKind must accept uppercase plurals: the old TrimSuffix-before-
// ToLower order meant 'S' != 's', so `ax delete TASKS foo` errored while
// `ax delete tasks foo` worked.
func TestNormalizeKindCase(t *testing.T) {
	cases := []struct {
		in       string
		wantKind string
		wantErr  bool
	}{
		{"task", "Task", false},
		{"tasks", "Task", false},
		{"TASKS", "Task", false},
		{"Task", "Task", false},
		{"GATEWAYS", "Gateway", false},
		{"Workspaces", "Workspace", false},
		{"MODELS", "Model", false},
		{"frobnicate", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := normalizeKind(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeKind(%q) = %q, nil; want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeKind(%q) error = %v", tc.in, err)
			}
			if got != tc.wantKind {
				t.Fatalf("normalizeKind(%q) = %q; want %q", tc.in, got, tc.wantKind)
			}
		})
	}
}
