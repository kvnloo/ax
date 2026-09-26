package main

import (
	"strings"
	"testing"
)

// The version line must describe what the binary actually is: a CLI client
// against an AX server. "standalone redis engine" predated the
// client-server restructure and told users the opposite of the truth.
func TestAxVersionLineDescribesCLI(t *testing.T) {
	line := axVersionLine()
	if !strings.HasPrefix(line, "ax version ") {
		t.Fatalf("version line %q does not start with %q", line, "ax version ")
	}
	if strings.Contains(line, "standalone") || strings.Contains(line, "redis") {
		t.Fatalf("version line %q still claims a bundled engine", line)
	}
}
