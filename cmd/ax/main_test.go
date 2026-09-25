package main

import "testing"

func TestParseGetArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    getTarget
		wantErr bool
	}{
		// The reported bug: plural resource plus a name silently listed
		// everything instead of fetching the named resource.
		{"plural task with name", []string{"tasks", "mytask"}, getTarget{kind: "task", name: "mytask"}, false},
		{"plural gateway with name", []string{"gateways", "my-gw"}, getTarget{kind: "gateway", name: "my-gw"}, false},
		{"plural workspace with name", []string{"workspaces", "my-ws"}, getTarget{kind: "workspace", name: "my-ws"}, false},
		{"plural model with name", []string{"models", "my-model"}, getTarget{kind: "model", name: "my-model"}, false},
		// Singular forms keep working.
		{"singular task with name", []string{"task", "mytask"}, getTarget{kind: "task", name: "mytask"}, false},
		{"singular gateway with name", []string{"gateway", "my-gw"}, getTarget{kind: "gateway", name: "my-gw"}, false},
		{"singular workspace with name", []string{"workspace", "my-ws"}, getTarget{kind: "workspace", name: "my-ws"}, false},
		{"singular model with name", []string{"model", "my-model"}, getTarget{kind: "model", name: "my-model"}, false},
		// Bare resource lists.
		{"plural task lists", []string{"tasks"}, getTarget{kind: "task"}, false},
		{"singular task lists", []string{"task"}, getTarget{kind: "task"}, false},
		{"plural gateway lists", []string{"gateways"}, getTarget{kind: "gateway"}, false},
		{"plural workspace lists", []string{"workspaces"}, getTarget{kind: "workspace"}, false},
		{"plural model lists", []string{"models"}, getTarget{kind: "model"}, false},
		// Case-insensitive like before.
		{"uppercase lists", []string{"TASKS"}, getTarget{kind: "task"}, false},
		{"mixed case with name", []string{"Gateways", "my-gw"}, getTarget{kind: "gateway", name: "my-gw"}, false},
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
