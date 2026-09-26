package tunnel

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A kubectl that prints the forwarding line and then dies frees its local
// port. If the OS recycles the port for an unrelated service answering 200
// on /healthz, spawnTunnel must not hand that URL back as ours: the health
// loop has to notice the child is gone, the same way the port-wait loop
// already does with its childExited probe.
func TestSpawnTunnelHealthLoopNoticesDeadChild(t *testing.T) {
	// Stub HTTP server answering 200 on /healthz: the foreign service on
	// the recycled port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})}
	defer srv.Close()
	go srv.Serve(ln)

	// Fake kubectl: prints the forwarding line for the stub's port, then
	// exits 1 — the "target service vanished right after the forward
	// started" shape.
	bin := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\necho 'Forwarding from 127.0.0.1:%d -> 8080'\nexit 1\n", port)
	if err := os.WriteFile(filepath.Join(bin, "kubectl"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AX_HOME", t.TempDir())

	dir, err := TunnelDir()
	if err != nil {
		t.Fatal(err)
	}

	_, err = spawnTunnel("deadchild", dir, Options{Namespace: "ax-system", Service: "ax-server", Port: 8080})
	if err == nil {
		t.Fatal("spawnTunnel returned a URL for a port-forward whose kubectl already exited; want an error")
	}
	if !strings.Contains(err.Error(), "exited") {
		t.Fatalf("error %q does not mention the exited child", err)
	}
}
