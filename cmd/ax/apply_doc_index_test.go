package main

import (
	"context"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
)

// fakeApplyClient is a minimal AXClient for apply-path tests: only CreateTask
// is implemented; every other method comes from the embedded nil interface
// and would panic if called.
type fakeApplyClient struct {
	v1alpha1.AXClient
	created []*v1alpha1.Task
}

func (f *fakeApplyClient) CreateTask(ctx context.Context, req *v1alpha1.CreateTaskRequest, _ ...grpc.CallOption) (*v1alpha1.Task, error) {
	f.created = append(f.created, req.Task)
	return req.Task, nil
}

func TestManifestDocsSkipsBlankSeparators(t *testing.T) {
	data := "---\nkind: Task\nmetadata:\n  name: a\n---\n---\nkind: Task\nmetadata:\n  name: b\n"
	docs, err := manifestDocs([]byte(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2 (blank separator must not count)", len(docs))
	}
}

func TestManifestDocsDecodeErrorSurfaces(t *testing.T) {
	// A syntax error anywhere in the stream must surface as an error.
	// (Which document number it reports is best-effort: yaml.v3's scanner
	// reads ahead, so a later-stream syntax error can surface on an
	// earlier Decode call. Only the "applying document N" numbering, which
	// is computed after successful decodes, is pinned to real documents.)
	data := "---\nkind: Task\nmetadata:\n  name: a\n---\n---\n\t: bad\n"
	if _, err := manifestDocs([]byte(data)); err == nil {
		t.Fatal("expected decode error, got nil")
	}
}

func TestApplyManifestDocIndexCountsRealDocs(t *testing.T) {
	// The second real document has an unsupported kind. The blank "---"
	// between the documents must not shift the reported document number:
	// the failure is on document 2, not 3.
	data := "---\nkind: Task\nmetadata:\n  name: a\n---\n---\nkind: Bogus\n"
	client := &fakeApplyClient{}
	err := applyManifest(context.Background(), client, []byte(data))
	if err == nil {
		t.Fatal("expected error for unsupported kind, got nil")
	}
	if !strings.Contains(err.Error(), "applying document 2:") {
		t.Errorf("error %q does not report real-document index 2", err.Error())
	}
	if len(client.created) != 1 {
		t.Errorf("created %d tasks, want 1 (first doc applies before the failure)", len(client.created))
	}
}

func TestApplyManifestAllValid(t *testing.T) {
	data := "---\nkind: Task\nmetadata:\n  name: a\n---\nkind: Task\nmetadata:\n  name: b\n"
	client := &fakeApplyClient{}
	if err := applyManifest(context.Background(), client, []byte(data)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.created) != 2 {
		t.Errorf("created %d tasks, want 2", len(client.created))
	}
}
