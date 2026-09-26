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

package tunnel

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

// procAvailable reports whether /proc-based command-line inspection works
// (Linux). Elsewhere the identity check falls back to liveness only.
func procAvailable() bool {
	_, err := os.Stat("/proc/self/cmdline")
	return err == nil
}

// startForeignHealthz serves 200 on /healthz from an unrelated local
// service, simulating a recycled tunnel port.
func startForeignHealthz(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})}
	go srv.Serve(ln) //nolint:errcheck
	t.Cleanup(func() { _ = srv.Close() })
	return port
}

// startFakeKubectl starts a long-lived process whose cmdline carries the
// kubectl port-forward signature. (A copied sleep binary run with
// port-forward args would exit immediately on the invalid interval, leaving
// a zombie whose empty cmdline fails the identity check.)
func startFakeKubectl(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("bash", "-c", "exec -a 'kubectl port-forward' sleep 300")
	if err := cmd.Start(); err != nil {
		t.Skipf("bash unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd
}

func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "300")
	if err := cmd.Start(); err != nil {
		t.Skipf("sleep unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd
}

// The health check alone is fooled by a foreign service on a recycled port:
// this documents the defect the Active verdict must not inherit.
func TestHealthCheckAloneFooledByForeignService(t *testing.T) {
	port := startForeignHealthz(t)
	if !IsTunnelHealthy(port) {
		t.Fatalf("IsTunnelHealthy(%d) = false, want true (foreign 200 on /healthz)", port)
	}
}

// A genuine tunnel — our port-forward process plus a answering healthz —
// must read Active.
func TestIsTunnelActiveGenuine(t *testing.T) {
	port := startForeignHealthz(t) // stands in for the real ax-server here
	kubectl := startFakeKubectl(t)
	info := &TunnelInfo{Context: "ctx", Port: port, PID: kubectl.Process.Pid}
	if !IsTunnelActive(info) {
		t.Fatalf("IsTunnelActive = false for genuine port-forward, want true")
	}
}

// Foreign service answering 200 on a recycled port, recorded process dead:
// must NOT read Active.
func TestIsTunnelActiveForeignServiceDeadProcess(t *testing.T) {
	port := startForeignHealthz(t)
	sleeper := startSleeper(t)
	pid := sleeper.Process.Pid
	_ = sleeper.Process.Kill()
	_ = sleeper.Wait() // reaped: sig-0 no longer fooled
	deadline := time.Now().Add(2 * time.Second)
	for processAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	info := &TunnelInfo{Context: "ctx", Port: port, PID: pid}
	if IsTunnelActive(info) {
		t.Fatalf("IsTunnelActive = true for dead process + foreign 200, want false")
	}
}

// Foreign service answering 200, recorded PID recycled by an unrelated live
// process: must NOT read Active (Linux /proc cmdline check).
func TestIsTunnelActiveForeignServiceRecycledPID(t *testing.T) {
	if !procAvailable() {
		t.Skip("needs /proc cmdline inspection")
	}
	port := startForeignHealthz(t)
	sleeper := startSleeper(t) // plain sleep: cmdline has no kubectl marker
	info := &TunnelInfo{Context: "ctx", Port: port, PID: sleeper.Process.Pid}
	if IsTunnelActive(info) {
		t.Fatalf("IsTunnelActive = true for unrelated process + foreign 200, want false")
	}
}

func TestIsTunnelActiveNoHealthz(t *testing.T) {
	kubectl := startFakeKubectl(t)
	// Port 1 is (practically) never a live healthz endpoint.
	info := &TunnelInfo{Context: "ctx", Port: 1, PID: kubectl.Process.Pid}
	if IsTunnelActive(info) {
		t.Fatalf("IsTunnelActive = true with no healthz responder, want false")
	}
}

func TestIsTunnelActiveNilAndZero(t *testing.T) {
	if IsTunnelActive(nil) {
		t.Fatalf("IsTunnelActive(nil) = true, want false")
	}
	if IsTunnelActive(&TunnelInfo{Context: "ctx", Port: 8080, PID: 0}) {
		t.Fatalf("IsTunnelActive(zero PID) = true, want false")
	}
}
