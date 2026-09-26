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
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/ax/internal/server"
	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/pkg/apis/v1alpha1"
)

// captureStderr runs fn with os.Stderr redirected to a pipe and returns
// what fn printed there.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// The "Watching task ..." banner is a diagnostic, not event data: it must
// go to stderr so the event stream on stdout stays clean for piping,
// matching delete's "waiting for ... to be deleted..." progress line.
func TestRunWatch_BannerGoesToStderr(t *testing.T) {
	memStore := memory.NewStore()
	srv := server.NewServer(memStore)
	ctx := context.Background()
	// A terminal-phase task ends the stream after the initial update, so
	// runWatch returns without hanging.
	if err := memStore.SaveTask(ctx, &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "w1", Atespace: "default"},
		Status:   &v1alpha1.TaskStatus{Phase: "Completed"},
	}); err != nil {
		t.Fatalf("seeding task: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	httpServer := &http.Server{Handler: srv.Handler()}
	httpServer.Protocols = new(http.Protocols)
	httpServer.Protocols.SetHTTP1(true)
	httpServer.Protocols.SetUnencryptedHTTP2(true)
	go func() { _ = httpServer.Serve(ln) }()
	defer httpServer.Close()

	var runErr error
	stdout := captureStdout(t, func() {
		stderr := captureStderr(t, func() {
			runErr = runWatch("http://"+ln.Addr().String(), "default", []string{"task", "w1"})
		})
		if !strings.Contains(stderr, "Watching task default/w1...") {
			t.Errorf("stderr = %q, want the watch banner on stderr", stderr)
		}
	})
	if runErr != nil {
		t.Fatalf("runWatch: %v", runErr)
	}
	if strings.Contains(stdout, "Watching task") {
		t.Errorf("stdout = %q, banner must not pollute the event stream", stdout)
	}
	if !strings.Contains(stdout, "Completed") {
		t.Errorf("stdout = %q, want the terminal phase event on stdout", stdout)
	}
}
