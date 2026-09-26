package guest

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestWaitReadyClosedConnReturns: on a closed connection WaitReady must
// return promptly instead of blocking until ctx expires. The pre-fix code
// looped on WaitForStateChange, which never fires for a Shutdown connection,
// so a non-expiring ctx hung forever, contradicting WaitReady's contract.
func TestWaitReadyClosedConnReturns(t *testing.T) {
	// grpc.NewClient is lazy: dialing an unlistened port never errors here.
	c, err := Dial("127.0.0.1:1")
	if err != nil {
		t.Fatalf("dialing: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("closing: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- c.WaitReady(context.Background()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error waiting on a closed connection")
		}
		if !strings.Contains(err.Error(), "closed") {
			t.Fatalf("error should name the closed connection, got: %q", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitReady hung on a closed connection with a non-expiring context")
	}
}

// TestWaitReadyNilConnFailsClearly: a zero-value Client has no connection.
func TestWaitReadyNilConnFailsClearly(t *testing.T) {
	c := &Client{}
	if err := c.WaitReady(context.Background()); err == nil {
		t.Fatal("expected an error with no connection")
	}
}
