package main

import (
	"strings"
	"testing"
)

// The short = forms (`-a=prod`, `-n=ns`) that Go's flag package and kubectl
// accept. The old parser only matched bare `-a`/`-n`, so the attached form
// fell into cleanArgs and blew up downstream in normalizeKind with a
// misleading "unsupported kind".
func TestParseGlobalArgsShortEqualsAtespace(t *testing.T) {
	cmd, cleanArgs, atespace, _, _, _, explicit, err := parseGlobalArgs(
		[]string{"get", "-a=prod", "tasks"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "get" {
		t.Fatalf("cmd = %q, want get", cmd)
	}
	if atespace != "prod" || !explicit {
		t.Fatalf("atespace = %q explicit=%v, want prod/true", atespace, explicit)
	}
	if strings.Join(cleanArgs, "\x00") != "tasks" {
		t.Fatalf("cleanArgs = %q, want [tasks]", cleanArgs)
	}
}

func TestParseGlobalArgsShortEqualsNamespace(t *testing.T) {
	_, _, _, _, _, axNamespace, _, err := parseGlobalArgs(
		[]string{"-n=prod-ns", "tunnel", "list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if axNamespace != "prod-ns" {
		t.Fatalf("axNamespace = %q, want prod-ns", axNamespace)
	}
}

func TestParseGlobalArgsShortEqualsValueWithEquals(t *testing.T) {
	_, _, atespace, _, _, _, _, err := parseGlobalArgs(
		[]string{"get", "-a=pro=d", "tasks"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atespace != "pro=d" {
		t.Fatalf("atespace = %q, want pro=d", atespace)
	}
}
