package main

import "testing"

// TestDisplayBranch: whitespace-only counts as unset and reads "main",
// matching the runner's workspace setup default. Without the whitespace
// fold, `ax describe workspace` rendered the raw " " for a repo the runner
// could never clone (git fetch fails on a " " branch).
func TestDisplayBranch(t *testing.T) {
	for in, want := range map[string]string{
		"":              "main",
		"   ":           "main",
		"main":          "main",
		"release-1.2":   "release-1.2",
		" release-1.2 ": "release-1.2",
	} {
		if got := displayBranch(in); got != want {
			t.Errorf("displayBranch(%q) = %q, want %q", in, got, want)
		}
	}
}
