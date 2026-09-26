package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The --file=<path> and -f=<path> forms must read the manifest like the
// separate-token forms. The old parser only matched bare "-f"/"--file", so
// `ax apply --file=foo.yaml` fell through to "missing required flag: -f
// <file>" even though the user did pass --file. validateApplyPositionals
// needs no change: the =value form consumes no following arg, so it already
// passes through as a flag.
func TestManifestFromArgsEqualsForms(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "m.yaml")
	content := "kind: Task\nmetadata:\n  name: demo\nspec: {}\n"
	if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--file=" + f},
		{"-f=" + f},
		{"--file", f},
		{"-f", f},
	} {
		data, ok, err := manifestFromArgs(args)
		if err != nil || !ok {
			t.Fatalf("manifestFromArgs(%q) = ok=%v err=%v, want the manifest", args, ok, err)
		}
		if string(data) != content {
			t.Fatalf("manifestFromArgs(%q) read %q, want the file content", args, data)
		}
	}
}

// --file= with nothing attached is the same missing-value error as a
// trailing bare -f.
func TestManifestFromArgsEqualsFormEmpty(t *testing.T) {
	if _, _, err := manifestFromArgs([]string{"--file="}); err == nil {
		t.Fatal("manifestFromArgs(--file=) = nil error, want missing-value error")
	}
	if _, _, err := manifestFromArgs([]string{"-f"}); err == nil {
		t.Fatal("manifestFromArgs(-f) = nil error, want missing-value error")
	}
	if _, ok, err := manifestFromArgs([]string{"get"}); err != nil || ok {
		t.Fatalf("manifestFromArgs(get) = ok=%v err=%v, want ok=false", ok, err)
	}
}

// The =value form must not eat the next arg as its value.
func TestManifestFromArgsEqualsFormSkipsNothing(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "m.yaml")
	if err := os.WriteFile(f, []byte("kind: Task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A positional after --file=<path> is still the validator's problem, not
	// the file parser's: manifestFromArgs must return after the first -f.
	data, ok, err := manifestFromArgs([]string{"--file=" + f, "extra"})
	if err != nil || !ok || len(data) == 0 {
		t.Fatalf("manifestFromArgs(--file=<path>, extra) = ok=%v err=%v, want the manifest", ok, err)
	}
	if err := validateApplyPositionals([]string{"--file=" + f, "extra"}); err == nil {
		t.Fatal("validateApplyPositionals(--file=<path>, extra) = nil, want extra-arg error")
	}
}
