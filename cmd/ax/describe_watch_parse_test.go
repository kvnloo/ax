package main

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

func TestParseDescribeArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantKind string
		wantName string
		wantErr  bool
	}{
		{"task lowercase", []string{"task", "foo"}, v1alpha1.KindTask, "foo", false},
		{"task plural", []string{"tasks", "foo"}, v1alpha1.KindTask, "foo", false},
		{"task capitalized", []string{"Task", "foo"}, v1alpha1.KindTask, "foo", false},
		{"task upper plural", []string{"TASKS", "foo"}, v1alpha1.KindTask, "foo", false},
		{"workspace upper plural", []string{"WORKSPACES", "foo"}, v1alpha1.KindWorkspace, "foo", false},
		{"model mixed plural", []string{"Models", "foo"}, v1alpha1.KindModel, "foo", false},
		{"unknown kind errors", []string{"banana", "foo"}, "", "", true},
		{"unknown kind typo errors", []string{"taks", "foo"}, "", "", true},
		{"missing name", []string{"task"}, "", "", true},
		{"no args", nil, "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, name, err := parseDescribeArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", kind, tc.wantKind)
			}
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
		})
	}
}

func TestParseWatchArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantName string
		wantErr  bool
	}{
		{"task kind", []string{"task", "foo"}, "foo", false},
		{"task plural kind", []string{"tasks", "foo"}, "foo", false},
		{"task capitalized", []string{"Task", "foo"}, "foo", false},
		// The defect: args[0] was ignored entirely, so a typo'd kind
		// silently watched the task anyway.
		{"unknown kind errors", []string{"banana", "foo"}, "", true},
		{"workspace kind errors", []string{"workspace", "foo"}, "", true},
		{"model kind errors", []string{"model", "foo"}, "", true},
		{"missing name", []string{"task"}, "", true},
		{"bare name", []string{"foo"}, "", true},
		{"no args", nil, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, err := parseWatchArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
		})
	}
}
