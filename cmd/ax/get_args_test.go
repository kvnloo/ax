package main

import "testing"

// parseGetArgs must reject the defects in the old inline conditions:
//  1. && / || precedence made `ax get tasks foo` silently list all tasks
//     (the plural branch fired before the name was ever considered).
//  2. `ax get task foo bar` silently dropped the extra arg.
func TestParseGetArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr bool
		list    bool
		kind    string
		resName string
	}{
		{"no args", nil, true, false, "", ""},
		{"list plural tasks", []string{"tasks"}, false, true, "task", ""},
		{"list singular task", []string{"task"}, false, true, "task", ""},
		{"list gateways", []string{"gateways"}, false, true, "gateway", ""},
		{"list workspace", []string{"workspace"}, false, true, "workspace", ""},
		{"list models", []string{"models"}, false, true, "model", ""},
		{"get one task", []string{"task", "foo"}, false, false, "task", "foo"},
		{"get one gateway", []string{"gateway", "gw1"}, false, false, "gateway", "gw1"},
		{"plural with name is an error, not a silent list",
			[]string{"tasks", "foo"}, true, false, "", ""},
		{"plural gateways with name is an error",
			[]string{"gateways", "gw1"}, true, false, "", ""},
		{"extra args are an error, not silently dropped",
			[]string{"task", "foo", "bar"}, true, false, "", ""},
		{"extra args on list are an error",
			[]string{"tasks", "foo", "bar"}, true, false, "", ""},
		{"unknown kind", []string{"frobnicate"}, true, false, "", ""},
		{"unknown kind with name", []string{"frobnicate", "x"}, true, false, "", ""},
		{"case-insensitive kind", []string{"TASKS"}, false, true, "task", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op, err := parseGetArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseGetArgs(%v) = %+v, nil; want error", tc.args, op)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGetArgs(%v) error = %v; want %+v", tc.args, err, tc)
			}
			if op.list != tc.list || op.kind != tc.kind || op.name != tc.resName {
				t.Fatalf("parseGetArgs(%v) = %+v; want list=%v kind=%q name=%q",
					tc.args, op, tc.list, tc.kind, tc.resName)
			}
		})
	}
}
