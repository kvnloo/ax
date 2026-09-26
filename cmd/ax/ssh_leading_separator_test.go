package main

import (
	"reflect"
	"testing"
)

// parseGlobalArgs keeps a bare "--" in cleanArgs, so runSSH used to take it
// as the task name: `ax ssh -- mytask` tried to fetch a task literally named
// "--" instead of "mytask". The leading separator must be stripped before
// the task name is read.
func TestSSHTaskAndCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantTask string
		wantCmd  []string
	}{
		{"plain", []string{"mytask"}, "mytask", []string{"/bin/sh"}},
		{"command", []string{"mytask", "ls", "-la"}, "mytask", []string{"ls", "-la"}},
		{"command separator", []string{"mytask", "--", "env", "--server"}, "mytask", []string{"env", "--server"}},
		{"leading separator", []string{"--", "mytask"}, "mytask", []string{"/bin/sh"}},
		{"leading separator with command", []string{"--", "mytask", "ls"}, "mytask", []string{"ls"}},
		{"leading separator with command separator", []string{"--", "mytask", "--", "env"}, "mytask", []string{"env"}},
		{"lone separator", []string{"--"}, "", nil},
		{"empty", nil, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, cmd := sshTaskAndCommand(tt.args)
			if task != tt.wantTask {
				t.Errorf("sshTaskAndCommand(%q) task = %q, want %q", tt.args, task, tt.wantTask)
			}
			if !reflect.DeepEqual(cmd, tt.wantCmd) {
				t.Errorf("sshTaskAndCommand(%q) cmd = %q, want %q", tt.args, cmd, tt.wantCmd)
			}
		})
	}
}
