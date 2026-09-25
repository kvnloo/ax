package main

import (
	"strings"
	"testing"
)

// Item 10: `ax tunnel stop ctx1 ctx2` silently stops only ctx1 and
// `ax tunnel list extra` silently ignores the extra arg — the same
// silent-drop class as item 8, in the last unaudited command.

func TestCheckTunnelArgs(t *testing.T) {
	if err := checkTunnelArgs([]string{"list", "extra"}); err == nil {
		t.Fatal("expected error for extra arg to list, got nil")
	} else if !strings.Contains(err.Error(), `"extra"`) {
		t.Fatalf("error should name the extra arg, got: %v", err)
	}
	if err := checkTunnelArgs([]string{"stop", "ctx1", "ctx2"}); err == nil {
		t.Fatal("expected error for extra arg to stop, got nil")
	} else if !strings.Contains(err.Error(), `"ctx2"`) {
		t.Fatalf("error should name the extra arg, got: %v", err)
	}
	if err := checkTunnelArgs(nil); err == nil {
		t.Fatal("expected usage error for no args, got nil")
	}
	if err := checkTunnelArgs([]string{"bogus"}); err == nil {
		t.Fatal("expected error for unknown subcommand, got nil")
	}
	for _, args := range [][]string{{"list"}, {"stop"}, {"stop", "ctx1"}} {
		if err := checkTunnelArgs(args); err != nil {
			t.Fatalf("valid shape %v should pass: %v", args, err)
		}
	}
}

// Wiring: runTunnel must surface the arity error before touching tunnels.
func TestRunTunnelRejectsExtraArgsWithoutSideEffects(t *testing.T) {
	if err := runTunnel([]string{"stop", "ctx1", "ctx2"}); err == nil {
		t.Fatal("expected extra-arg error, got nil")
	} else if !strings.Contains(err.Error(), `"ctx2"`) {
		t.Fatalf("error should name the extra arg, got: %v", err)
	}
	if err := runTunnel([]string{"list", "extra"}); err == nil {
		t.Fatal("expected extra-arg error, got nil")
	}
	if err := runTunnel([]string{"bogus"}); err == nil {
		t.Fatal("expected unknown-subcommand error, got nil")
	}
}
