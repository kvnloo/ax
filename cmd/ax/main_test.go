// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeAXClient embeds the generated client so tests only override the RPCs
// they exercise.
type fakeAXClient struct {
	v1alpha1.AXClient
	getTaskErr   error
	updatedTasks []*v1alpha1.Task
}

func (f *fakeAXClient) GetTask(ctx context.Context, in *v1alpha1.GetTaskRequest, opts ...grpc.CallOption) (*v1alpha1.Task, error) {
	if f.getTaskErr != nil {
		return nil, f.getTaskErr
	}
	return &v1alpha1.Task{}, nil
}

func (f *fakeAXClient) UpdateTask(ctx context.Context, in *v1alpha1.UpdateTaskRequest, opts ...grpc.CallOption) (*v1alpha1.Task, error) {
	f.updatedTasks = append(f.updatedTasks, in.Task)
	return in.Task, nil
}

const multiDocManifest = `kind: Task
metadata:
  name: first
  atespace: default
spec: {}
---
---
kind: Bogus
metadata:
  name: second
  atespace: default
spec: {}
`

// A bare "---" between documents decodes to a null node, not an empty
// document node. It must be skipped, not reported as "missing kind", and it
// must not shift the reported index of later documents: the failing doc here
// is the second real document.
func TestApplyManifestsEmptyDocIndex(t *testing.T) {
	fc := &fakeAXClient{getTaskErr: status.Error(codes.NotFound, "no such task")}
	err := applyManifests(context.Background(), fc, []byte(multiDocManifest))
	if err == nil {
		t.Fatal("expected an error from the unsupported-kind document, got nil")
	}
	if !strings.Contains(err.Error(), "applying document 2:") {
		t.Fatalf("expected the failure to be reported as document 2, got: %v", err)
	}
	if strings.Contains(err.Error(), "missing kind") {
		t.Fatalf("the empty separator document must be skipped, got: %v", err)
	}
	// The first document applied before the failure.
	if len(fc.updatedTasks) != 1 || fc.updatedTasks[0].GetMetadata().GetName() != "first" {
		t.Fatalf("expected the first document to be applied before the failure, updated=%v", fc.updatedTasks)
	}
}

// All-valid multi-doc manifests apply in order with no error.
func TestApplyManifestsAllValid(t *testing.T) {
	manifest := `kind: Task
metadata:
  name: one
  atespace: default
spec: {}
---
kind: Task
metadata:
  name: two
  atespace: default
spec: {}
`
	fc := &fakeAXClient{getTaskErr: status.Error(codes.NotFound, "no such task")}
	if err := applyManifests(context.Background(), fc, []byte(manifest)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fc.updatedTasks) != 2 {
		t.Fatalf("expected 2 UpdateTask calls, got %d", len(fc.updatedTasks))
	}
}

// A manifest with only blank separators between valid docs must apply every
// real document — separators are not failures.
func TestApplyManifestsBlankSeparators(t *testing.T) {
	manifest := `---
kind: Task
metadata:
  name: one
  atespace: default
spec: {}
---
---
kind: Task
metadata:
  name: two
  atespace: default
spec: {}
---
`
	fc := &fakeAXClient{getTaskErr: status.Error(codes.NotFound, "no such task")}
	if err := applyManifests(context.Background(), fc, []byte(manifest)); err != nil {
		t.Fatalf("blank separators must not fail the apply, got: %v", err)
	}
	if len(fc.updatedTasks) != 2 {
		t.Fatalf("expected 2 UpdateTask calls, got %d", len(fc.updatedTasks))
	}
}

// releaseSSHSession must invoke the port-forward cleanup and every closer,
// tolerating a nil cleanup (direct-reachability path).
func TestReleaseSSHSessionRunsAll(t *testing.T) {
	var order []string
	releaseSSHSession(
		func() { order = append(order, "cleanup") },
		func() error { order = append(order, "guest"); return nil },
		func() error { order = append(order, "conn"); return errors.New("boom") },
	)
	if strings.Join(order, ",") != "cleanup,guest,conn" {
		t.Fatalf("order = %v, want all three invoked in order", order)
	}
	releaseSSHSession(nil, func() error { order = append(order, "only"); return nil })
	if order[len(order)-1] != "only" {
		t.Fatalf("nil cleanup must be tolerated, order = %v", order)
	}
}

// The non-zero remote exit path in runSSH calls os.Exit, which never runs
// deferred calls. This test forks a child exercising the two patterns:
// the pre-fix pattern (defer cleanup; os.Exit) must leave the marker absent
// (the leak), while the fixed pattern (explicit releaseSSHSession before
// os.Exit) must leave it present and still exit with the remote status.
func TestRemoteExitCleanupMechanic(t *testing.T) {
	if mode := os.Getenv("AX_SSH_EXIT_CHILD"); mode != "" {
		marker := os.Getenv("AX_SSH_EXIT_MARKER")
		cleanup := func() { _ = os.WriteFile(marker, []byte("cleaned"), 0644) }
		if mode == "old" {
			defer cleanup()
			os.Exit(3) // pre-fix pattern: deferred cleanup never runs
		}
		releaseSSHSession(cleanup, func() error { return nil })
		os.Exit(3) // fixed pattern: cleanup runs, exit status preserved
	}

	for _, tc := range []struct {
		mode        string
		wantMarker  bool
		description string
	}{
		{"old", false, "pre-fix defer+os.Exit pattern must skip cleanup (the leak)"},
		{"new", true, "fixed explicit-release pattern must run cleanup before exiting"},
	} {
		marker := filepath.Join(t.TempDir(), "cleaned")
		cmd := exec.Command(os.Args[0], "-test.run=TestRemoteExitCleanupMechanic")
		cmd.Env = append(os.Environ(),
			"AX_SSH_EXIT_CHILD="+tc.mode, "AX_SSH_EXIT_MARKER="+marker)
		err := cmd.Run()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
			t.Fatalf("%s: child exit = %v, want status 3", tc.description, err)
		}
		_, statErr := os.Stat(marker)
		if tc.wantMarker && statErr != nil {
			t.Fatalf("%s: marker absent, cleanup did not run", tc.description)
		}
		if !tc.wantMarker && statErr == nil {
			t.Fatalf("%s: marker present, expected the leak", tc.description)
		}
	}
}

// A bare "--" ends global option parsing: words after it are positional even
// when they look like global flags, so the remote command of
// `ax ssh mytask -- env --server` reaches the guest intact. On the old
// parser the trailing "--server" was silently swallowed and "--context=prod"
// was consumed as the CLI's own kube context.
func TestParseGlobalArgsDashDashPassthrough(t *testing.T) {
	cmd, cleanArgs, _, explicitServer, kubeContext, _, err := parseGlobalArgs(
		[]string{"ssh", "mytask", "--", "env", "--server"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "ssh" {
		t.Fatalf("cmd = %q, want ssh", cmd)
	}
	if explicitServer != "" {
		t.Fatalf("explicitServer = %q, want empty (flag belongs to the remote command)", explicitServer)
	}
	want := []string{"mytask", "--", "env", "--server"}
	if strings.Join(cleanArgs, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("cleanArgs = %q, want %q", cleanArgs, want)
	}

	var err2 error
	_, cleanArgs, _, _, kubeContext, _, err2 = parseGlobalArgs(
		[]string{"ssh", "mytask", "--", "echo", "--context=prod"})
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if kubeContext != "" {
		t.Fatalf("kubeContext = %q, want empty (flag belongs to the remote command)", kubeContext)
	}
	want = []string{"mytask", "--", "echo", "--context=prod"}
	if strings.Join(cleanArgs, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("cleanArgs = %q, want %q", cleanArgs, want)
	}
}

// Global flags before the command still parse as before.
func TestParseGlobalArgsFlagsBeforeCommand(t *testing.T) {
	cmd, cleanArgs, atespace, explicitServer, kubeContext, axNamespace, err := parseGlobalArgs(
		[]string{"--server=http://x:1", "--context", "prod", "get", "tasks"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "get" || explicitServer != "http://x:1" || kubeContext != "prod" {
		t.Fatalf("got cmd=%q server=%q context=%q", cmd, explicitServer, kubeContext)
	}
	if atespace != "default" || axNamespace != "ax-system" {
		t.Fatalf("defaults changed: atespace=%q namespace=%q", atespace, axNamespace)
	}
	if len(cleanArgs) != 1 || cleanArgs[0] != "tasks" {
		t.Fatalf("cleanArgs = %q, want [tasks]", cleanArgs)
	}
}

// `ax tunnel list foo` and `ax tunnel stop ctx extra` silently ignore the
// extra args today: list prints the table regardless, stop stops `ctx` and
// drops `extra`. Both must be usage errors, like the other ax commands.
func TestRunTunnelListExtraArgs(t *testing.T) {
	if err := runTunnel([]string{"list", "foo"}); err == nil {
		t.Fatalf("runTunnel(list, foo) = nil, want usage error")
	} else if !strings.Contains(err.Error(), "usage") {
		t.Fatalf("runTunnel(list, foo) error = %q, want usage error", err)
	}
}

func TestRunTunnelStopExtraArgs(t *testing.T) {
	err := runTunnel([]string{"stop", "foo", "extra"})
	if err == nil {
		t.Fatalf("runTunnel(stop, foo, extra) = nil, want usage error")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Fatalf("runTunnel(stop, foo, extra) error = %q, want usage error", err)
	}
}

// `ax suspend task foo bar` and `ax resume foo bar extra` silently ignored the
// trailing args and suspended/resumed the first name. Anything beyond the
// bare-name or kind-name form is a usage error.
func TestParseSuspendResumeName(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"foo"}, "foo"},
		{[]string{"task", "foo"}, "foo"},
		{[]string{"tasks", "foo"}, "foo"},
		{[]string{"foo", "bar"}, "foo"}, // legacy positional form
	}
	for _, tc := range cases {
		got, err := parseSuspendResumeName("suspend", tc.args)
		if err != nil || got != tc.want {
			t.Errorf("parseSuspendResumeName(suspend, %q) = %q, %v; want %q, nil", tc.args, got, err, tc.want)
		}
	}
	for _, args := range [][]string{{}, {"task", "foo", "bar"}, {"foo", "bar", "baz"}} {
		if name, err := parseSuspendResumeName("resume", args); err == nil {
			t.Errorf("parseSuspendResumeName(resume, %q) = %q, nil; want usage error", args, name)
		}
	}
}

// A value-taking global flag as the last word silently keeps its default
// today: `ax get tasks --server` swallows the flag and the CLI tunnels to
// kube instead of failing. A trailing valueless flag must be an error. The
// `=value` forms and the post-`--` passthrough are unaffected.
func TestParseGlobalArgsTrailingFlagNeedsValue(t *testing.T) {
	for _, flag := range []string{"-a", "--atespace", "--server", "--context", "-n", "--namespace"} {
		_, _, _, _, _, _, err := parseGlobalArgs([]string{"get", "tasks", flag})
		if err == nil {
			t.Errorf("parseGlobalArgs(..., %q) = nil error, want missing-value error", flag)
		}
	}

	// =value forms still parse; post--- words stay positional.
	_, _, atespace, _, _, _, err := parseGlobalArgs([]string{"get", "tasks", "--atespace=prod"})
	if err != nil || atespace != "prod" {
		t.Errorf("=value form broke: atespace=%q err=%v", atespace, err)
	}
	_, cleanArgs, _, _, _, _, err := parseGlobalArgs([]string{"ssh", "mytask", "--", "--server"})
	if err != nil {
		t.Errorf("post--- passthrough broke: err=%v", err)
	}
	if len(cleanArgs) != 3 || cleanArgs[2] != "--server" {
		t.Errorf("cleanArgs = %q, want [mytask -- --server]", cleanArgs)
	}
}
