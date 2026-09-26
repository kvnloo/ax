package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

// namelessApplyClient records whether the server was ever contacted, so the
// client-side rejection is observable without a server.
type namelessApplyClient struct {
	v1alpha1.AXClient
	getTasks int
	updated  int
}

func (c *namelessApplyClient) GetTask(ctx context.Context, in *v1alpha1.GetTaskRequest, opts ...grpc.CallOption) (*v1alpha1.Task, error) {
	c.getTasks++
	return nil, status.Error(codes.NotFound, "no such task")
}

func (c *namelessApplyClient) UpdateTask(ctx context.Context, in *v1alpha1.UpdateTaskRequest, opts ...grpc.CallOption) (*v1alpha1.Task, error) {
	c.updated++
	return in.Task, nil
}

func applyNamelessDoc(t *testing.T, client v1alpha1.AXClient, manifest string) (kind, name, outcome string, err error) {
	t.Helper()
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader([]byte(manifest)))
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("decoding test manifest: %v", err)
	}
	return applyDocument(context.Background(), client, &doc, "default", false)
}

// A manifest document without metadata.name used to dial the server and
// submit a nameless resource: GetTask("") missed, UpdateTask persisted a
// nameless record, and the CLI printed `task.ax.io/ created`. It must fail
// client-side before any RPC.
func TestApplyDocumentRejectsMissingName(t *testing.T) {
	fc := &namelessApplyClient{}
	_, _, _, err := applyNamelessDoc(t, fc, "kind: Task\nspec: {}\n")
	if err == nil || !strings.Contains(err.Error(), "metadata.name") {
		t.Fatalf("applyDocument(nameless) = %v, want missing metadata.name error", err)
	}
	if fc.getTasks != 0 || fc.updated != 0 {
		t.Fatalf("nameless document must not reach the server: get=%d update=%d",
			fc.getTasks, fc.updated)
	}
}

// A document with an empty metadata block is the same defect.
func TestApplyDocumentRejectsEmptyMetadata(t *testing.T) {
	fc := &namelessApplyClient{}
	_, _, _, err := applyNamelessDoc(t, fc, "kind: Task\nmetadata: {}\nspec: {}\n")
	if err == nil || !strings.Contains(err.Error(), "metadata.name") {
		t.Fatalf("applyDocument(empty metadata) = %v, want missing metadata.name error", err)
	}
	if fc.getTasks != 0 || fc.updated != 0 {
		t.Fatalf("nameless document must not reach the server: get=%d update=%d",
			fc.getTasks, fc.updated)
	}
}

// Named documents still apply exactly as before.
func TestApplyDocumentNamedStillApplies(t *testing.T) {
	fc := &namelessApplyClient{}
	kind, name, outcome, err := applyNamelessDoc(t, fc,
		"kind: Task\nmetadata:\n  name: demo\nspec: {}\n")
	if err != nil {
		t.Fatalf("applyDocument(named) = %v, want nil", err)
	}
	if kind != v1alpha1.KindTask || name != "demo" || outcome != "created" {
		t.Fatalf("applyDocument(named) = (%q, %q, %q), want (Task, demo, created)", kind, name, outcome)
	}
}
