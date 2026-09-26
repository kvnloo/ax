package tunnel

import "testing"

// A whitespace-only --server / AX_SERVER must count as unset, not as a URL:
// normalizeServerURL would trim it to "" and the CLI would dial an empty
// target instead of auto-detecting from the kube context. With no kube
// context available the fallback is the local default.
func TestEnsureServerURLWhitespaceOnlyFallsThrough(t *testing.T) {
	t.Setenv("AX_SERVER", " ")
	t.Setenv("KUBECONTEXT", "")
	t.Setenv("KUBECONFIG", "/nonexistent-kubeconfig")

	url, err := EnsureServerURL(Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://localhost:8080" {
		t.Fatalf("whitespace AX_SERVER must fall through to detection, got %q", url)
	}

	url, err = EnsureServerURL(Options{ServerURL: "  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://localhost:8080" {
		t.Fatalf("whitespace --server must fall through to detection, got %q", url)
	}
}

// A padded-but-real URL still wins over detection.
func TestEnsureServerURLPaddedValueKept(t *testing.T) {
	t.Setenv("AX_SERVER", "")

	url, err := EnsureServerURL(Options{ServerURL: "  http://h:8080/ "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://h:8080" {
		t.Fatalf("expected trimmed URL, got %q", url)
	}
}
