package main

import (
	"reflect"
	"testing"
)

func TestParseSSHCommand(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantTask string
		wantCmd  []string
		wantErr  bool
	}{
		{"empty args", nil, "", nil, true},
		{"bare task name", []string{"mytask"}, "mytask", nil, false},
		{"command words", []string{"mytask", "ls", "-la"}, "mytask", []string{"ls", "-la"}, false},
		{"dashdash only", []string{"mytask", "--", "-a"}, "mytask", []string{"-a"}, false},
		// The defect: words before "--" were silently dropped on the old inline loop.
		{"dashdash appends", []string{"mytask", "ls", "--", "-a"}, "mytask", []string{"ls", "-a"}, false},
		{"dashdash remote help", []string{"mytask", "echo", "--", "--help"}, "mytask", []string{"echo", "--help"}, false},
		{"second dashdash verbatim", []string{"mytask", "echo", "--", "--", "--help"}, "mytask", []string{"echo", "--", "--help"}, false},
		{"trailing dashdash", []string{"mytask", "ls", "--"}, "mytask", []string{"ls"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task, cmd, err := parseSSHCommand(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if task != tc.wantTask {
				t.Errorf("task = %q, want %q", task, tc.wantTask)
			}
			if !reflect.DeepEqual(cmd, tc.wantCmd) {
				t.Errorf("cmd = %q, want %q", cmd, tc.wantCmd)
			}
		})
	}
}
