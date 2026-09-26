package main

import (
	"context"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
)

// getArgFake records which RPCs runGetWithClient invokes, so tests can tell
// a list apart from a get-one (both succeed against the base implementation,
// but they must hit different methods).
type getArgFake struct {
	fakeAXClient
	gotTaskName    string
	gotGatewayName string
	listTaskCalls  int
}

func (f *getArgFake) GetTask(ctx context.Context, in *v1alpha1.GetTaskRequest, opts ...grpc.CallOption) (*v1alpha1.Task, error) {
	f.gotTaskName = in.GetName()
	return &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: in.GetName()}}, nil
}

func (f *getArgFake) ListTasks(ctx context.Context, in *v1alpha1.ListTasksRequest, opts ...grpc.CallOption) (*v1alpha1.ListTasksResponse, error) {
	f.listTaskCalls++
	return &v1alpha1.ListTasksResponse{}, nil
}

func (f *getArgFake) GetGateway(ctx context.Context, in *v1alpha1.GetGatewayRequest, opts ...grpc.CallOption) (*v1alpha1.Gateway, error) {
	f.gotGatewayName = in.GetName()
	return &v1alpha1.Gateway{}, nil
}

func (f *getArgFake) GetWorkspace(ctx context.Context, in *v1alpha1.GetWorkspaceRequest, opts ...grpc.CallOption) (*v1alpha1.Workspace, error) {
	return &v1alpha1.Workspace{}, nil
}

func (f *getArgFake) GetModel(ctx context.Context, in *v1alpha1.GetModelRequest, opts ...grpc.CallOption) (*v1alpha1.Model, error) {
	return &v1alpha1.Model{}, nil
}

// `ax get " Task"` must list tasks: the resource position normalizes like
// describe/watch/delete/apply, so whitespace and capitalization route to
// the same kind instead of failing with "unknown resource".
func TestGetNormalizesKindPosition(t *testing.T) {
	fc := &getArgFake{}
	if err := runGetWithClient(context.Background(), fc, "default", []string{" Task"}); err != nil {
		t.Fatalf("runGetWithClient: %v", err)
	}
	if fc.listTaskCalls != 1 {
		t.Errorf("expected one ListTasks call, got %d", fc.listTaskCalls)
	}
}
// list branch matched `resource == "tasks"` with any arg count, shadowing the
// get-one branch below it.
func TestGetTasksPluralWithNameRoutesToGetOne(t *testing.T) {
	fc := &getArgFake{}
	if err := runGetWithClient(context.Background(), fc, "default", []string{"tasks", "foo"}); err != nil {
		t.Fatalf("runGetWithClient: %v", err)
	}
	if fc.gotTaskName != "foo" {
		t.Errorf("expected GetTask(\"foo\"), got name %q (listCalls=%d)", fc.gotTaskName, fc.listTaskCalls)
	}
	if fc.listTaskCalls != 0 {
		t.Errorf("ListTasks must not run for a named get, calls=%d", fc.listTaskCalls)
	}
}

// `ax get task foo bar` must fail fast instead of silently dropping "bar".
// describe/delete already reject extra positionals this way.
func TestGetTaskExtraArgRejected(t *testing.T) {
	fc := &getArgFake{}
	err := runGetWithClient(context.Background(), fc, "default", []string{"task", "foo", "bar"})
	if err == nil || !strings.Contains(err.Error(), `unexpected extra argument "bar"`) {
		t.Fatalf("runGetWithClient(task foo bar) err = %v, want unexpected extra argument", err)
	}
	if fc.gotTaskName != "" || fc.listTaskCalls != 0 {
		t.Errorf("no RPC may run when args are invalid (gotTask=%q listCalls=%d)", fc.gotTaskName, fc.listTaskCalls)
	}
}

func TestGetTasksExtraArgRejected(t *testing.T) {
	fc := &getArgFake{}
	err := runGetWithClient(context.Background(), fc, "default", []string{"tasks", "foo", "bar"})
	if err == nil || !strings.Contains(err.Error(), `unexpected extra argument "bar"`) {
		t.Fatalf("runGetWithClient(tasks foo bar) err = %v, want unexpected extra argument", err)
	}
}

func TestGetGatewayExtraArgRejected(t *testing.T) {
	fc := &getArgFake{}
	err := runGetWithClient(context.Background(), fc, "default", []string{"gateway", "foo", "bar"})
	if err == nil || !strings.Contains(err.Error(), `unexpected extra argument "bar"`) {
		t.Fatalf("runGetWithClient(gateway foo bar) err = %v, want unexpected extra argument", err)
	}
	if fc.gotGatewayName != "" {
		t.Errorf("no RPC may run when args are invalid (gotGateway=%q)", fc.gotGatewayName)
	}
}

func TestGetWorkspaceExtraArgRejected(t *testing.T) {
	fc := &getArgFake{}
	err := runGetWithClient(context.Background(), fc, "default", []string{"workspace", "foo", "bar"})
	if err == nil || !strings.Contains(err.Error(), `unexpected extra argument "bar"`) {
		t.Fatalf("runGetWithClient(workspace foo bar) err = %v, want unexpected extra argument", err)
	}
}

func TestGetModelExtraArgRejected(t *testing.T) {
	fc := &getArgFake{}
	err := runGetWithClient(context.Background(), fc, "default", []string{"model", "foo", "bar"})
	if err == nil || !strings.Contains(err.Error(), `unexpected extra argument "bar"`) {
		t.Fatalf("runGetWithClient(model foo bar) err = %v, want unexpected extra argument", err)
	}
}
