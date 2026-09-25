// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "m.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("writing test manifest: %v", err)
	}
	return p
}

// A second positional manifest was silently ignored: only the -f file was
// applied, the rest never read.
func TestManifestFromArgsStrayPositional(t *testing.T) {
	a := writeManifest(t, "kind: Task\n")
	b := writeManifest(t, "kind: Task\n")
	_, ok, err := manifestFromArgs([]string{"-f", a, b})
	if err == nil {
		t.Fatalf("expected error for stray positional %q, got ok=%v", b, ok)
	}
	if !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("expected unexpected-argument error, got %v", err)
	}
}

// A lone positional with no -f at all must not be silently ignored either.
func TestManifestFromArgsBarePositional(t *testing.T) {
	a := writeManifest(t, "kind: Task\n")
	if _, _, err := manifestFromArgs([]string{a}); err == nil {
		t.Fatalf("expected error for bare positional manifest, got nil")
	}
}

// Duplicate -f flags must keep erroring (chunk-3 behavior, preserved here).
func TestManifestFromArgsDuplicateFileFlag(t *testing.T) {
	a := writeManifest(t, "kind: Task\n")
	b := writeManifest(t, "kind: Task\n")
	if _, _, err := manifestFromArgs([]string{"-f", a, "-f", b}); err == nil {
		t.Fatalf("expected error for duplicate -f, got nil")
	}
}

// --file=<path> matches the CLI's --atespace= convention.
func TestManifestFromArgsFileEqualsForm(t *testing.T) {
	a := writeManifest(t, "kind: Task\n")
	data, ok, err := manifestFromArgs([]string{"--file=" + a})
	if err != nil || !ok {
		t.Fatalf("--file=<path>: got ok=%v err=%v", ok, err)
	}
	if !strings.Contains(string(data), "kind: Task") {
		t.Fatalf("--file=<path>: unexpected data %q", data)
	}
}

// Unchanged behavior pins.
func TestManifestFromArgsPins(t *testing.T) {
	a := writeManifest(t, "kind: Task\n")
	if _, ok, err := manifestFromArgs(nil); err != nil || ok {
		t.Fatalf("no args: got ok=%v err=%v", ok, err)
	}
	if _, _, err := manifestFromArgs([]string{"-f"}); err == nil {
		t.Fatalf("bare -f: expected error, got nil")
	}
	data, ok, err := manifestFromArgs([]string{"-f", a})
	if err != nil || !ok || !strings.Contains(string(data), "kind: Task") {
		t.Fatalf("-f <file>: got ok=%v err=%v data=%q", ok, err, data)
	}
}
