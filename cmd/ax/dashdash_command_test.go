package main

import "testing"

// `ax -- get tasks` used to report "unknown command" because the --
// separator path never captured the command word. The command after "--"
// is still the command.
func TestParseGlobalArgsDashDashCapturesCommand(t *testing.T) {
	cmd, cleanArgs, _, _, _, _, _, err := parseGlobalArgs([]string{"--", "get", "tasks"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "get" {
		t.Fatalf("expected cmd %q, got %q", "get", cmd)
	}
	if len(cleanArgs) != 1 || cleanArgs[0] != "tasks" {
		t.Fatalf("expected cleanArgs [tasks], got %v", cleanArgs)
	}
}

// Flags before the command still work, and a leading "--" before an ssh
// command is untouched: sshTaskAndCommand still sees the separator.
func TestParseGlobalArgsDashDashBeforeSSH(t *testing.T) {
	cmd, cleanArgs, _, _, _, _, _, err := parseGlobalArgs([]string{"--", "ssh", "mytask"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "ssh" {
		t.Fatalf("expected cmd %q, got %q", "ssh", cmd)
	}
	if len(cleanArgs) != 1 || cleanArgs[0] != "mytask" {
		t.Fatalf("expected cleanArgs [mytask], got %v", cleanArgs)
	}
	task, _ := sshTaskAndCommand(cleanArgs)
	if task != "mytask" {
		t.Fatalf("expected task %q, got %q", "mytask", task)
	}
}

// A "--" after the command keeps its old behavior: it stays in cleanArgs.
func TestParseGlobalArgsDashDashAfterCommandUnchanged(t *testing.T) {
	cmd, cleanArgs, _, _, _, _, _, err := parseGlobalArgs([]string{"ssh", "--", "mytask"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "ssh" {
		t.Fatalf("expected cmd %q, got %q", "ssh", cmd)
	}
	task, _ := sshTaskAndCommand(cleanArgs)
	if task != "mytask" {
		t.Fatalf("expected task %q, got %q", "mytask", task)
	}
}
