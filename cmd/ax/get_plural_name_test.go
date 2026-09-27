package main

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/google/ax/internal/server"
	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

// testServer starts an in-process AX API server over bufconn and routes the
// CLI's gRPC dials to it. It returns the seeded store and a teardown func.
func testServer(t *testing.T) (*memory.MemoryStore, func()) {
	t.Helper()

	st := memory.NewStore()
	ctx := context.Background()
	for _, name := range []string{"foo", "bar"} {
		if err := st.SaveTask(ctx, &v1alpha1.Task{
			Metadata: &v1alpha1.ObjectMeta{Name: name, Atespace: "default"},
		}); err != nil {
			t.Fatalf("seeding task %q: %v", name, err)
		}
	}

	lis := bufconn.Listen(1024 * 1024)
	srv := server.NewServer(st)
	go func() { _ = srv.GRPCServer().Serve(lis) }()

	old := axDialOptions
	axDialOptions = []grpc.DialOption{
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
	}
	return st, func() {
		axDialOptions = old
		srv.GRPCServer().Stop()
		_ = lis.Close()
	}
}

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runErr := fn()

	_ = w.Close()
	os.Stdout = old
	out := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, rerr := r.Read(buf)
		out = append(out, buf[:n]...)
		if rerr != nil {
			break
		}
	}
	_ = r.Close()
	return string(out), runErr
}

func TestRunGetPluralKindWithNameFetchesSingle(t *testing.T) {
	_, teardown := testServer(t)
	defer teardown()

	out, err := captureStdout(t, func() error {
		return runGet("bufnet", "default", []string{"tasks", "foo"})
	})
	if err != nil {
		t.Fatalf("runGet: %v", err)
	}
	// Single-get renders the one task as YAML; the other task must not appear.
	if !strings.Contains(out, "foo") {
		t.Errorf("output missing requested task foo:\n%s", out)
	}
	if strings.Contains(out, "bar") {
		t.Errorf("ax get tasks foo listed other tasks instead of fetching foo:\n%s", out)
	}
}

func TestRunGetPluralKindWithoutNameStillLists(t *testing.T) {
	_, teardown := testServer(t)
	defer teardown()

	out, err := captureStdout(t, func() error {
		return runGet("bufnet", "default", []string{"tasks"})
	})
	if err != nil {
		t.Fatalf("runGet: %v", err)
	}
	if !strings.Contains(out, "foo") || !strings.Contains(out, "bar") {
		t.Errorf("ax get tasks should list all tasks, got:\n%s", out)
	}
}
