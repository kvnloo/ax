package main

import (
	"strings"
	"testing"
)

// Item 8: extra positional args are silently dropped by the named-resource
// commands. These tests pin strict arity: `ax describe task foo bar` must
// error instead of describing foo and ignoring bar.

func TestCheckGetArgs(t *testing.T) {
	if err := checkGetArgs([]string{"task", "foo", "bar"}); err == nil {
		t.Fatal("expected error for extra arg, got nil")
	} else if !strings.Contains(err.Error(), `"bar"`) {
		t.Fatalf("error should name the extra arg, got: %v", err)
	}
	if err := checkGetArgs(nil); err == nil {
		t.Fatal("expected usage error for no args, got nil")
	}
	if err := checkGetArgs([]string{"tasks"}); err != nil {
		t.Fatalf("list form should still work: %v", err)
	}
	if err := checkGetArgs([]string{"task", "foo"}); err != nil {
		t.Fatalf("get form should still work: %v", err)
	}
}

func TestCheckDescribeArgs(t *testing.T) {
	if err := checkDescribeArgs([]string{"task", "foo", "bar"}); err == nil {
		t.Fatal("expected error for extra arg, got nil")
	} else if !strings.Contains(err.Error(), `"bar"`) {
		t.Fatalf("error should name the extra arg, got: %v", err)
	}
	if err := checkDescribeArgs([]string{"task"}); err == nil {
		t.Fatal("expected usage error for missing name, got nil")
	}
	if err := checkDescribeArgs([]string{"task", "foo"}); err != nil {
		t.Fatalf("exact arity should pass: %v", err)
	}
}

func TestCheckDeleteArgs(t *testing.T) {
	if err := checkDeleteArgs([]string{"task", "foo", "bar"}); err == nil {
		t.Fatal("expected error for extra arg, got nil")
	}
	if err := checkDeleteArgs([]string{"task"}); err == nil {
		t.Fatal("expected usage error for missing name, got nil")
	}
	if err := checkDeleteArgs([]string{"gateway", "gw1"}); err != nil {
		t.Fatalf("exact arity should pass: %v", err)
	}
}

func TestCheckWatchArgs(t *testing.T) {
	if err := checkWatchArgs([]string{"task", "foo", "bar"}); err == nil {
		t.Fatal("expected error for extra arg, got nil")
	}
	if err := checkWatchArgs([]string{"task"}); err == nil {
		t.Fatal("expected usage error for missing name, got nil")
	}
	if err := checkWatchArgs([]string{"task", "foo"}); err != nil {
		t.Fatalf("exact arity should pass: %v", err)
	}
}

func TestCheckSuspendArgs(t *testing.T) {
	// Both call shapes reject a third arg.
	for _, args := range [][]string{{"foo", "bar", "baz"}, {"task", "foo", "bar"}} {
		if err := checkSuspendArgs(args); err == nil {
			t.Fatalf("expected error for extra arg in %v, got nil", args)
		} else if !strings.Contains(err.Error(), `"bar"`) && !strings.Contains(err.Error(), `"baz"`) {
			t.Fatalf("error should name the extra arg, got: %v", err)
		}
	}
	if err := checkSuspendArgs(nil); err == nil {
		t.Fatal("expected usage error for no args, got nil")
	}
	// Both valid shapes still pass.
	if err := checkSuspendArgs([]string{"foo"}); err != nil {
		t.Fatalf("bare name should pass: %v", err)
	}
	if err := checkSuspendArgs([]string{"task", "foo"}); err != nil {
		t.Fatalf("kind+name should pass: %v", err)
	}
}

func TestCheckResumeArgs(t *testing.T) {
	if err := checkResumeArgs([]string{"foo", "bar", "baz"}); err == nil {
		t.Fatal("expected error for extra arg, got nil")
	}
	if err := checkResumeArgs(nil); err == nil {
		t.Fatal("expected usage error for no args, got nil")
	}
	if err := checkResumeArgs([]string{"tasks", "foo"}); err != nil {
		t.Fatalf("kind+name should pass: %v", err)
	}
}

// Wiring: the run* functions must surface the arity error before dialing.
func TestRunFunctionsRejectExtraArgsBeforeDial(t *testing.T) {
	const srv, at = "http://127.0.0.1:1", "default"
	cases := []struct {
		name string
		fn   func() error
		want string
	}{
		{"get", func() error { return runGet(srv, at, []string{"task", "foo", "bar"}) }, `"bar"`},
		{"describe", func() error { return runDescribe(srv, at, []string{"task", "foo", "bar"}) }, `"bar"`},
		{"delete", func() error { return runDelete(srv, at, []string{"task", "foo", "bar"}) }, `"bar"`},
		{"watch", func() error { return runWatch(srv, at, []string{"task", "foo", "bar"}) }, `"bar"`},
		{"suspend", func() error { return runSuspend(srv, at, []string{"foo", "bar", "baz"}) }, `"baz"`},
		{"resume", func() error { return runResume(srv, at, []string{"task", "foo", "bar"}) }, `"bar"`},
	}
	for _, c := range cases {
		err := c.fn()
		if err == nil {
			t.Fatalf("%s: expected extra-arg error, got nil", c.name)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: error should name the extra arg, got: %v", c.name, err)
		}
	}
}
