package main

import (
	"strings"
	"testing"
)

// `ax watch gateway foo` used to ignore the kind entirely and try to watch a
// task named "foo", failing later with a confusing not-found. Only tasks are
// watchable; anything else must usage-error before dialing.
func TestRunWatchRejectsNonTaskKind(t *testing.T) {
	err := runWatch("http://127.0.0.1:1", "ns", []string{"gateway", "foo"})
	if err == nil || !strings.Contains(err.Error(), "usage: ax watch task <name>") {
		t.Fatalf("runWatch accepted kind %q, want a usage error; got %v", "gateway", err)
	}
}

// `ax watch task foo extra` used to silently drop "extra" and watch "foo".
func TestRunWatchRejectsExtraArgs(t *testing.T) {
	err := runWatch("http://127.0.0.1:1", "ns", []string{"task", "foo", "extra"})
	if err == nil || !strings.Contains(err.Error(), "unexpected extra argument") || !strings.Contains(err.Error(), "extra") {
		t.Fatalf("runWatch accepted an extra positional arg; got %v", err)
	}
}

// Valid args still proceed to the watch RPC (here: a refused connection,
// which must surface as a watch error, not a usage error).
func TestRunWatchValidArgsDial(t *testing.T) {
	err := runWatch("http://127.0.0.1:1", "ns", []string{"task", "foo"})
	if err == nil || strings.Contains(err.Error(), "usage:") {
		t.Fatalf("runWatch with valid args returned a usage error: %v", err)
	}
}
