package main

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// Ctrl-C during `ax ssh` used to kill the process without running deferred
// cleanup, orphaning the kubectl port-forward. The interrupt watcher must
// release the session and exit with the conventional 128+signum code.
func TestWatchSSHInterruptReleasesOnSIGINT(t *testing.T) {
	released := make(chan struct{}, 1)
	exited := make(chan int, 1)
	stop := watchSSHInterrupt(func() { released <- struct{}{} }, func(code int) { exited <- code })
	defer stop()

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("sending SIGINT to self: %v", err)
	}

	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("release func not called on SIGINT")
	}
	select {
	case code := <-exited:
		if code != 130 {
			t.Fatalf("exit code = %d, want 130 for SIGINT", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("exit func not called on SIGINT")
	}
}

func TestExitCodeForSignal(t *testing.T) {
	if got := exitCodeForSignal(syscall.SIGINT); got != 130 {
		t.Errorf("SIGINT -> %d, want 130", got)
	}
	if got := exitCodeForSignal(syscall.SIGTERM); got != 143 {
		t.Errorf("SIGTERM -> %d, want 143", got)
	}
	// The handler only notifies on SIGINT/SIGTERM; anything else maps to the
	// SIGINT convention rather than 0.
	var _ os.Signal = syscall.SIGINT
}
