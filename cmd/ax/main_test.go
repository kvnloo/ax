package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Item 9: `ax apply -f a.yaml -f b.yaml` silently applies only a.yaml —
// manifestFromArgs returns on the first -f match and never looks at the
// second. A scripted multi-file apply silently drops every file after the
// first. Now a duplicate -f/--file is an error.

func writeTempManifest(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestManifestFromArgsRejectsDuplicateFileFlag(t *testing.T) {
	a := writeTempManifest(t, "a.yaml", "kind: Task\n")
	b := writeTempManifest(t, "b.yaml", "kind: Task\n")
	_, _, err := manifestFromArgs([]string{"-f", a, "-f", b})
	if err == nil {
		t.Fatal("expected error for duplicate -f, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error should say duplicate, got: %v", err)
	}
}

func TestManifestFromArgsRejectsMixedDuplicateFileFlag(t *testing.T) {
	a := writeTempManifest(t, "a.yaml", "kind: Task\n")
	b := writeTempManifest(t, "b.yaml", "kind: Task\n")
	// -f and --file are the same flag; mixing them still duplicates.
	if _, _, err := manifestFromArgs([]string{"-f", a, "--file", b}); err == nil {
		t.Fatal("expected error for -f + --file duplicate, got nil")
	}
}

func TestManifestFromArgsSingleFileStillWorks(t *testing.T) {
	a := writeTempManifest(t, "a.yaml", "kind: Task\n")
	data, ok, err := manifestFromArgs([]string{"-f", a})
	if err != nil {
		t.Fatalf("single -f should work: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for single -f")
	}
	if !strings.Contains(string(data), "kind: Task") {
		t.Fatalf("expected file content back, got %q", data)
	}
}

func TestManifestFromArgsKeepsExistingBehavior(t *testing.T) {
	// No -f at all.
	_, ok, err := manifestFromArgs([]string{"other"})
	if err != nil || ok {
		t.Fatalf("no -f should give ok=false, nil error; got ok=%v err=%v", ok, err)
	}
	// -f with no following value.
	if _, _, err := manifestFromArgs([]string{"-f"}); err == nil {
		t.Fatal("expected error for bare -f, got nil")
	}
}
