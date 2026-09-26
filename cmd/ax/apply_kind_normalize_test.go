package main

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

// fakeApplyClient answers task lookups with NotFound and echoes updates, so
// applyDocument can run end to end without a server.
type fakeApplyClient struct {
	v1alpha1.AXClient
	updated *v1alpha1.Task
}

func (f *fakeApplyClient) GetTask(ctx context.Context, in *v1alpha1.GetTaskRequest, opts ...grpc.CallOption) (*v1alpha1.Task, error) {
	return nil, status.Error(codes.NotFound, "no such task")
}

func (f *fakeApplyClient) UpdateTask(ctx context.Context, in *v1alpha1.UpdateTaskRequest, opts ...grpc.CallOption) (*v1alpha1.Task, error) {
	f.updated = in.Task
	return in.Task, nil
}

func applyDoc(t *testing.T, client v1alpha1.AXClient, manifest string) (kind, name, outcome string, err error) {
	t.Helper()
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader([]byte(manifest)))
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("decoding test manifest: %v", err)
	}
	return applyDocument(context.Background(), client, &doc)
}

const applyTaskManifest = `
kind: %s
metadata:
  name: demo
  atespace: default
spec: {}
`

// Manifest kinds must be normalized the same way user-typed kinds are for
// get/describe/delete: `kind: task` (lowercase, as users write it) was
// rejected by apply while every other command accepted it.
func TestApplyDocumentNormalizesKind(t *testing.T) {
	for _, tc := range []struct {
		rawKind  string
		wantKind string
	}{
		{"task", v1alpha1.KindTask},
		{"Task", v1alpha1.KindTask},
		{"tasks", v1alpha1.KindTask},
		{"TASK", v1alpha1.KindTask},
	} {
		fc := &fakeApplyClient{}
		kind, name, outcome, err := applyDoc(t, fc, fmt.Sprintf(applyTaskManifest, tc.rawKind))
		if err != nil {
			t.Errorf("applyDocument(kind %q): %v", tc.rawKind, err)
			continue
		}
		if kind != tc.wantKind {
			t.Errorf("applyDocument(kind %q) kind = %q, want %q", tc.rawKind, kind, tc.wantKind)
		}
		if name != "demo" || outcome != "created" {
			t.Errorf("applyDocument(kind %q) = (%q, %q), want (demo, created)", tc.rawKind, name, outcome)
		}
		if fc.updated == nil {
			t.Errorf("applyDocument(kind %q): UpdateTask was not called", tc.rawKind)
		}
	}
}

func TestApplyDocumentRejectsUnknownKind(t *testing.T) {
	fc := &fakeApplyClient{}
	if _, _, _, err := applyDoc(t, fc, fmt.Sprintf(applyTaskManifest, "bogus")); err == nil {
		t.Fatal("applyDocument(kind bogus): want error, got nil")
	}
	if _, _, _, err := applyDoc(t, fc, "metadata:\n  name: x\n"); err == nil {
		t.Fatal("applyDocument(missing kind): want error, got nil")
	}
}
