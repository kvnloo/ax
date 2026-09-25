package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempManifest(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestManifestFromArgsFileEquals(t *testing.T) {
	p := writeTempManifest(t, "kind: task\n")
	data, ok, err := manifestFromArgs([]string{"--file=" + p})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for --file=<path>")
	}
	if string(data) != "kind: task\n" {
		t.Fatalf("unexpected data: %q", data)
	}
}

func TestManifestFromArgsFileEqualsEmpty(t *testing.T) {
	_, _, err := manifestFromArgs([]string{"--file="})
	if err == nil {
		t.Fatal("expected error for empty --file=")
	}
}

func TestManifestFromArgsSpaceFormStillWorks(t *testing.T) {
	p := writeTempManifest(t, "kind: task\n")
	data, ok, err := manifestFromArgs([]string{"-f", p})
	if err != nil || !ok || string(data) != "kind: task\n" {
		t.Fatalf("space form regressed: ok=%v err=%v data=%q", ok, err, data)
	}
}
