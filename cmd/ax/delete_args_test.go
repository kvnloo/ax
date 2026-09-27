package main

import "testing"

// parseDeleteArgs must reject `ax delete task foo bar`, which silently
// dropped "bar" under the old `len(args) < 2` check.
func TestParseDeleteArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantErr  bool
		wantKind string
		wantName string
	}{
		{"task", []string{"task", "foo"}, false, "Task", "foo"},
		{"tasks plural", []string{"tasks", "foo"}, false, "Task", "foo"},
		{"gateway", []string{"gateway", "gw1"}, false, "Gateway", "gw1"},
		{"workspace", []string{"workspace", "w1"}, false, "Workspace", "w1"},
		{"model", []string{"model", "m1"}, false, "Model", "m1"},
		{"unknown kind", []string{"frobnicate", "x"}, true, "", ""},
		{"missing name", []string{"task"}, true, "", ""},
		{"no args", nil, true, "", ""},
		{"extra args are an error, not silently dropped",
			[]string{"task", "foo", "bar"}, true, "", ""},
		{"extra args gateway", []string{"gateway", "g", "x"}, true, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, name, err := parseDeleteArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseDeleteArgs(%v) = (%q, %q), nil; want error", tc.args, kind, name)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDeleteArgs(%v) error = %v", tc.args, err)
			}
			if kind != tc.wantKind || name != tc.wantName {
				t.Fatalf("parseDeleteArgs(%v) = (%q, %q); want (%q, %q)",
					tc.args, kind, name, tc.wantKind, tc.wantName)
			}
		})
	}
}
