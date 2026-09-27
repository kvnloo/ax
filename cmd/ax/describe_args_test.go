package main

import "testing"

// parseDescribeArgs must reject the defects in the old inline handling:
//  1. unknown kinds (e.g. `ax describe frobnicate x`) fell through to a task
//     lookup instead of erroring;
//  2. `ax describe task foo bar` silently dropped the extra arg.
func TestParseDescribeArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantErr  bool
		wantKind string
		wantName string
	}{
		{"task", []string{"task", "foo"}, false, "task", "foo"},
		{"tasks plural", []string{"tasks", "foo"}, false, "tasks", "foo"},
		{"gateway", []string{"gateway", "gw1"}, false, "gateway", "gw1"},
		{"workspaces plural", []string{"workspaces", "w1"}, false, "workspaces", "w1"},
		{"model", []string{"model", "m1"}, false, "model", "m1"},
		{"case-insensitive", []string{"TASKS", "foo"}, false, "tasks", "foo"},
		{"unknown kind is an error, not a task fallthrough",
			[]string{"frobnicate", "x"}, true, "", ""},
		{"missing name", []string{"task"}, true, "", ""},
		{"no args", nil, true, "", ""},
		{"extra args are an error, not silently dropped",
			[]string{"task", "foo", "bar"}, true, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, name, err := parseDescribeArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseDescribeArgs(%v) = (%q, %q), nil; want error", tc.args, kind, name)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDescribeArgs(%v) error = %v", tc.args, err)
			}
			if kind != tc.wantKind || name != tc.wantName {
				t.Fatalf("parseDescribeArgs(%v) = (%q, %q); want (%q, %q)",
					tc.args, kind, name, tc.wantKind, tc.wantName)
			}
		})
	}
}
