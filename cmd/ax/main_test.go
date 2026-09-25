package main

import (
	"strings"
	"testing"
)

// Tests for parseGlobalFlags: the pure extraction of main()'s global flag
// loop. RED on base (parseGlobalFlags is undefined); GREEN after Item A.

func TestParseGlobalFlagsMissingValue(t *testing.T) {
	for _, args := range [][]string{
		{"-a"},
		{"--atespace"},
		{"--server"},
		{"--context"},
		{"-n"},
		{"--namespace"},
		{"get", "tasks", "-a"}, // trailing flag after the command
	} {
		_, _, _, err := parseGlobalFlags(args)
		if err == nil || !strings.Contains(err.Error(), "requires a value") {
			t.Errorf("parseGlobalFlags(%q): expected missing-value error, got %v", args, err)
		}
	}
}

func TestParseGlobalFlagsEmptyValue(t *testing.T) {
	for _, args := range [][]string{
		{"--atespace="},
		{"-a", ""},
		{"--server="},
		{"--namespace="},
		{"-n", ""},
	} {
		_, _, _, err := parseGlobalFlags(args)
		if err == nil || !strings.Contains(err.Error(), "non-empty") {
			t.Errorf("parseGlobalFlags(%q): expected empty-value error, got %v", args, err)
		}
	}
}

func TestParseGlobalFlagsShortEqualsForm(t *testing.T) {
	cmd, flags, clean, err := parseGlobalFlags([]string{"-a=team1", "get", "tasks"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "get" || flags.atespace != "team1" {
		t.Errorf("got cmd=%q atespace=%q, want get/team1", cmd, flags.atespace)
	}
	if len(clean) != 1 || clean[0] != "tasks" {
		t.Errorf("got cleanArgs=%q, want [tasks]", clean)
	}

	_, flags, _, err = parseGlobalFlags([]string{"-n=ns1", "version"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags.axNamespace != "ns1" {
		t.Errorf("got namespace=%q, want ns1", flags.axNamespace)
	}
}

func TestParseGlobalFlagsLongEqualsForm(t *testing.T) {
	_, flags, _, err := parseGlobalFlags([]string{"--atespace=team2", "--server=http://x:8080", "get", "tasks"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags.atespace != "team2" || flags.explicitServer != "http://x:8080" {
		t.Errorf("got %+v, want atespace=team2 server=http://x:8080", flags)
	}
}

func TestParseGlobalFlagsValueSwallowingNextFlag(t *testing.T) {
	// "-a --server=x" must not silently take "--server=x" as the atespace.
	_, _, _, err := parseGlobalFlags([]string{"-a", "--server=x", "get", "tasks"})
	if err == nil {
		t.Errorf("expected error when a flag value looks like another flag")
	}
}

func TestParseGlobalFlagsUnknownFlagBeforeCommand(t *testing.T) {
	_, _, _, err := parseGlobalFlags([]string{"--bogus", "get", "tasks"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected unknown-flag error, got %v", err)
	}
}

func TestParseGlobalFlagsUnknownFlagAfterCommandPassesThrough(t *testing.T) {
	// Command-level rejection (Item B) owns dash args after the command word.
	cmd, _, clean, err := parseGlobalFlags([]string{"get", "tasks", "--bogus"})
	if err != nil {
		t.Fatalf("parse-level must not reject post-command flags: %v", err)
	}
	if cmd != "get" || len(clean) != 2 || clean[1] != "--bogus" {
		t.Errorf("got cmd=%q clean=%q", cmd, clean)
	}
}

func TestParseGlobalFlagsHelp(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"-h", "get"}} {
		cmd, _, _, err := parseGlobalFlags(args)
		if err != nil {
			t.Fatalf("parseGlobalFlags(%q): unexpected error %v", args, err)
		}
		if cmd != "help" {
			t.Errorf("parseGlobalFlags(%q): got cmd=%q, want help", args, cmd)
		}
	}
}

func TestParseGlobalFlagsPostCommandGlobalsStillWork(t *testing.T) {
	// Pre-existing behavior: global flags work after the command word too.
	_, flags, clean, err := parseGlobalFlags([]string{"get", "tasks", "-a", "foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags.atespace != "foo" {
		t.Errorf("got atespace=%q, want foo", flags.atespace)
	}
	if len(clean) != 1 || clean[0] != "tasks" {
		t.Errorf("got clean=%q, want [tasks]", clean)
	}
}

func TestParseGlobalFlagsDefaultsAndLoneDash(t *testing.T) {
	_, flags, clean, err := parseGlobalFlags([]string{"-"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags.atespace != "default" || flags.axNamespace != "ax-system" {
		t.Errorf("defaults changed: %+v", flags)
	}
	if len(clean) != 1 || clean[0] != "-" {
		t.Errorf("lone dash must pass through, got %q", clean)
	}
}

func TestParseGlobalFlagsContextForms(t *testing.T) {
	_, flags, _, err := parseGlobalFlags([]string{"--context=myctx", "ctx"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags.kubeContext != "myctx" {
		t.Errorf("got context=%q, want myctx", flags.kubeContext)
	}
	_, flags, _, err = parseGlobalFlags([]string{"--context", "myctx", "ctx"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags.kubeContext != "myctx" {
		t.Errorf("got context=%q, want myctx", flags.kubeContext)
	}
}
