package main

import (
	"strings"
	"testing"
)

func TestRejectUnexpectedFlags(t *testing.T) {
	if err := rejectUnexpectedFlags([]string{"tasks"}); err != nil {
		t.Errorf("plain args must pass: %v", err)
	}
	if err := rejectUnexpectedFlags(nil); err != nil {
		t.Errorf("empty args must pass: %v", err)
	}
	if err := rejectUnexpectedFlags([]string{"-"}); err != nil {
		t.Errorf("lone dash (stdin) must pass: %v", err)
	}
	// Anything after "--" is literal passthrough.
	if err := rejectUnexpectedFlags([]string{"--", "--bogus"}); err != nil {
		t.Errorf("post-separator args must pass: %v", err)
	}
	for _, args := range [][]string{{"--bogus"}, {"tasks", "--bogus"}, {"-x"}} {
		err := rejectUnexpectedFlags(args)
		if err == nil || !strings.Contains(err.Error(), "unexpected flag") {
			t.Errorf("rejectUnexpectedFlags(%q): expected unexpected-flag error, got %v", args, err)
		}
	}
}

func TestManifestFromArgsRejectsUnknownFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--bogus", "-f", "x.yaml"},
		{"-f", "x.yaml", "--bogus"},
	} {
		_, _, err := manifestFromArgs(args)
		if err == nil || !strings.Contains(err.Error(), "unexpected flag") {
			t.Errorf("manifestFromArgs(%q): expected unexpected-flag error, got %v", args, err)
		}
	}
	// "-f -" (stdin) still passes the flag scan.
	_, _, err := manifestFromArgs([]string{"-"})
	if err != nil {
		t.Errorf("manifestFromArgs([-]): unexpected error %v", err)
	}
}

func TestCommandsRejectUnknownFlagsBeforeNetwork(t *testing.T) {
	// All of these must fail before any server connection is attempted.
	const bogus = "http://127.0.0.1:1"
	cases := []struct {
		name string
		fn   func() error
	}{
		{"get", func() error { return runGet(bogus, "default", []string{"tasks", "--bogus"}) }},
		{"describe", func() error { return runDescribe(bogus, "default", []string{"task", "x", "--bogus"}) }},
		{"watch", func() error { return runWatch(bogus, "default", []string{"task", "x", "--bogus"}) }},
		{"delete", func() error { return runDelete(bogus, "default", []string{"task", "x", "--bogus"}) }},
		{"suspend", func() error { return runSuspend(bogus, "default", []string{"--bogus"}) }},
		{"resume", func() error { return runResume(bogus, "default", []string{"x", "--bogus"}) }},
		{"ssh-task", func() error { return runSSH(bogus, "default", "", []string{"--bogus"}) }},
	}
	for _, c := range cases {
		err := c.fn()
		if err == nil || !strings.Contains(err.Error(), "unexpected flag") {
			t.Errorf("%s: expected unexpected-flag error, got %v", c.name, err)
		}
	}
}

func TestTunnelRejectsUnknownFlags(t *testing.T) {
	if err := runTunnel([]string{"list", "--bogus"}); err == nil ||
		!strings.Contains(err.Error(), "unexpected flag") {
		t.Errorf("tunnel list --bogus: expected unexpected-flag error, got %v", err)
	}
}
