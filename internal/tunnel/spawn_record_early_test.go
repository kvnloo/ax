package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A SIGKILL of the CLI between kubectl spawn and state recording used to
// orphan a stateless port-forward: no state file existed, so the next
// EnsureServerURL could neither find nor reap it. The state must be recorded
// as soon as the port assignment is known, before the health wait — so this
// test fails if the file only appears after spawnTunnel returns.
func TestSpawnTunnelRecordsStateBeforeHealthWait(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AX_HOME", home)

	// Fake kubectl: prints the forwarding line for a port nothing serves,
	// then sleeps so the child stays alive through the health window.
	bindir := t.TempDir()
	fake := "#!/bin/sh\n" +
		"echo 'Forwarding from 127.0.0.1:45991 -> 8080'\n" +
		"sleep 30\n"
	if err := os.WriteFile(filepath.Join(bindir, "kubectl"), []byte(fake), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bindir+string(os.PathListSeparator)+os.Getenv("PATH"))

	dir := filepath.Join(home, "tunnels")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(dir, "record-before-health.json")

	done := make(chan error, 1)
	go func() {
		_, err := spawnTunnel("record-before-health", dir, Options{Namespace: "ns", Service: "svc", Port: 8080})
		done <- err
	}()

	// The health wait runs ~3s and never passes (nothing serves :45991).
	// The state file must appear well before spawnTunnel gives up.
	appeared := false
	for i := 0; i < 40; i++ {
		if _, err := os.Stat(statePath); err == nil {
			appeared = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !appeared {
		// Drain the goroutine before failing so the fake kubectl is reaped.
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
		t.Fatal("state file not recorded before the health wait finished")
	}

	// The spawn must still fail (health never passed), and the failed
	// attempt must clean up the entry it recorded early.
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected spawnTunnel to fail on an unservable port")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("spawnTunnel did not return")
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state file left behind after failed spawn: %v", err)
	}
	fmt.Println("ok")
}
