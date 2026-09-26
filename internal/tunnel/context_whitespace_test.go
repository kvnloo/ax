package tunnel

import (
	"os"
	"path/filepath"
	"testing"
)

// A whitespace-only --context override is not a context name: it must fall
// through to KUBECONTEXT / kubeconfig instead of being returned verbatim
// (where it would be handed to kubectl --context=" " and to
// SanitizeContext, producing a bogus "_.json" state file).
func TestCurrentContextWhitespaceOverrideFallsThrough(t *testing.T) {
	dir := t.TempDir()
	kubeconfig := filepath.Join(dir, "config")
	content := "current-context: kube-real\ncontexts: []\nclusters: []\nusers: []\n"
	if err := os.WriteFile(kubeconfig, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
	t.Setenv("KUBECONTEXT", "")

	ctx, err := CurrentContext("   ")
	if err != nil {
		t.Fatalf("CurrentContext: %v", err)
	}
	if ctx != "kube-real" {
		t.Fatalf("whitespace-only override returned %q, want kubeconfig fallthrough %q", ctx, "kube-real")
	}
}

// The KUBECONTEXT env var gets the same treatment: blank means unset.
func TestCurrentContextWhitespaceEnvFallsThrough(t *testing.T) {
	dir := t.TempDir()
	kubeconfig := filepath.Join(dir, "config")
	content := "current-context: kube-real\ncontexts: []\nclusters: []\nusers: []\n"
	if err := os.WriteFile(kubeconfig, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
	t.Setenv("KUBECONTEXT", "  ")

	ctx, err := CurrentContext("")
	if err != nil {
		t.Fatalf("CurrentContext: %v", err)
	}
	if ctx != "kube-real" {
		t.Fatalf("whitespace-only KUBECONTEXT returned %q, want kubeconfig fallthrough %q", ctx, "kube-real")
	}
}

// A real override still wins verbatim.
func TestCurrentContextRealOverrideWins(t *testing.T) {
	t.Setenv("KUBECONTEXT", "env-ctx")
	ctx, err := CurrentContext("flag-ctx")
	if err != nil {
		t.Fatalf("CurrentContext: %v", err)
	}
	if ctx != "flag-ctx" {
		t.Fatalf("real override returned %q, want %q", ctx, "flag-ctx")
	}
}
