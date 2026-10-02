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

func TestApplyManifestSkipsBlankSeparators(t *testing.T) {
	data := "---\nkind: Task\nmetadata:\n  name: a\n---\n---\nkind: Task\nmetadata:\n  name: b\n"
	client := &fakeApplyClient{}
	if err := applyManifest(context.Background(), client, []byte(data)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.created) != 2 {
		t.Fatalf("created %d tasks, want 2 (blank separator must not count)", len(client.created))
	}
}

func TestApplyManifestDecodeErrorSurfaces(t *testing.T) {
	// yaml.v3 can read ahead, so syntax-error indices are best-effort.
	data := "---\nkind: Task\nmetadata:\n  name: a\n---\n---\n\t: bad\n"
	if err := applyManifest(context.Background(), &fakeApplyClient{}, []byte(data)); err == nil {
		t.Fatal("expected decode error, got nil")
	}
}

func TestApplyManifestKeepsSuccessfulDocumentsBeforeDecodeError(t *testing.T) {
	data := "kind: Task\nmetadata:\n  name: first\n---\n---\nkind: Task\nmetadata:\n  name: second\n---\n[\n"
	client := &fakeApplyClient{}
	err := applyManifest(context.Background(), client, []byte(data))
	if err == nil || !strings.Contains(err.Error(), "decoding document 3:") {
		t.Fatalf("expected decode error for third real document, got %v", err)
	}
	if len(client.created) != 2 {
		t.Fatalf("created %d tasks, want 2 before the decode error", len(client.created))
	}
	if client.created[0].Metadata.Name != "first" || client.created[1].Metadata.Name != "second" {
		t.Fatalf("tasks applied out of order: %v", client.created)
	}
}

func TestApplyManifestStopsBeforeLaterDecodeError(t *testing.T) {
	data := "kind: Bogus\n---\n[\n"
	err := applyManifest(context.Background(), &fakeApplyClient{}, []byte(data))
	if err == nil || !strings.Contains(err.Error(), "applying document 1:") {
		t.Fatalf("expected first document's apply error, got %v", err)
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
