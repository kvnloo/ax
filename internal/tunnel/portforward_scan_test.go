package tunnel

import (
	"io"
	"strings"
	"testing"
	"time"
)

// chunkReader yields its chunks one Read at a time, then EOF. It forces the
// multi-read path of scanPortForwardOutput: each Read returns a line that
// matches portForwardRegex, like kubectl reprinting the forwarding line or
// "Handling connection" traffic keeping the accumulated buffer matching.
type chunkReader struct {
	chunks []string
	i      int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.i >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.i])
	r.i++
	return n, nil
}

func TestScanPortForwardOutputSendsPortOnce(t *testing.T) {
	line := "Forwarding from 127.0.0.1:54321 -> 80\n"
	portChan := make(chan int, 1)
	errChan := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		scanPortForwardOutput(&chunkReader{chunks: []string{line, line}}, portChan, errChan)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("scanPortForwardOutput did not return: blocked resending the port on a channel nobody reads")
	}

	if got := <-portChan; got != 54321 {
		t.Fatalf("got port %d, want 54321", got)
	}
	select {
	case p := <-portChan:
		t.Fatalf("second port delivered (%d); only the first match may be sent", p)
	default:
	}
	select {
	case err := <-errChan:
		if !strings.Contains(err.Error(), "kubectl port-forward exited") {
			t.Fatalf("unexpected error: %v", err)
		}
	default:
		t.Fatal("expected the exit error after EOF")
	}
}

func TestScanPortForwardOutputReportsExitError(t *testing.T) {
	portChan := make(chan int, 1)
	errChan := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		scanPortForwardOutput(strings.NewReader("error: Service \"nope\" not found\n"), portChan, errChan)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("scanPortForwardOutput did not return")
	}
	select {
	case p := <-portChan:
		t.Fatalf("port %d delivered with no forwarding line present", p)
	default:
	}
	select {
	case err := <-errChan:
		if !strings.Contains(err.Error(), "not found") {
			t.Fatalf("error should carry kubectl's output, got: %v", err)
		}
	default:
		t.Fatal("expected the exit error")
	}
}
