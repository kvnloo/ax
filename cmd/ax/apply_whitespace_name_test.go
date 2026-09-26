package main

import (
	"strings"
	"testing"
)

// A whitespace-only metadata.name is the chunk-33 nameless defect one
// character wider: the old == "" check passed, GetTask(" ") missed,
// UpdateTask persisted a record named " ", and the CLI printed
// `task.ax.io/  created`. It must fail client-side before any RPC.
func TestApplyDocumentRejectsWhitespaceOnlyName(t *testing.T) {
	fc := &namelessApplyClient{}
	_, _, _, err := applyNamelessDoc(t, fc,
		"kind: Task\nmetadata:\n  name: \"   \"\nspec: {}\n")
	if err == nil || !strings.Contains(err.Error(), "metadata.name") {
		t.Fatalf("applyDocument(whitespace name) = %v, want missing metadata.name error", err)
	}
	if fc.getTasks != 0 || fc.updated != 0 {
		t.Fatalf("whitespace-named document must not reach the server: get=%d update=%d",
			fc.getTasks, fc.updated)
	}
}

// Tab-only names are the same defect.
func TestApplyDocumentRejectsTabOnlyName(t *testing.T) {
	fc := &namelessApplyClient{}
	_, _, _, err := applyNamelessDoc(t, fc,
		"kind: Task\nmetadata:\n  name: \"\\t\"\nspec: {}\n")
	if err == nil || !strings.Contains(err.Error(), "metadata.name") {
		t.Fatalf("applyDocument(tab name) = %v, want missing metadata.name error", err)
	}
	if fc.getTasks != 0 || fc.updated != 0 {
		t.Fatalf("tab-named document must not reach the server: get=%d update=%d",
			fc.getTasks, fc.updated)
	}
}

// Names with real content still apply exactly as before.
func TestApplyDocumentNamedWithContentStillApplies(t *testing.T) {
	fc := &namelessApplyClient{}
	kind, name, outcome, err := applyNamelessDoc(t, fc,
		"kind: Task\nmetadata:\n  name: demo-ws\nspec: {}\n")
	if err != nil {
		t.Fatalf("applyDocument(named) = %v, want nil", err)
	}
	if kind != "Task" || name != "demo-ws" || outcome != "created" {
		t.Fatalf("applyDocument(named) = (%s, %s, %s), want (Task, demo-ws, created)",
			kind, name, outcome)
	}
	if fc.getTasks != 1 || fc.updated != 1 {
		t.Fatalf("named document should hit the server once: get=%d update=%d",
			fc.getTasks, fc.updated)
	}
}
