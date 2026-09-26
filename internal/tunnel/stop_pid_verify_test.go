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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// withTempAXHome points AX_HOME at a temp dir for the duration of the test.
func withTempAXHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AX_HOME", dir)
	return dir
}

func processAlive(pid int) bool {
	// /proc-based so that unreaped zombies (still "killable" with sig 0)
	// do not read as alive.
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	s := string(data)
	i := strings.LastIndex(s, ")")
	if i < 0 {
		return false
	}
	rest := strings.TrimSpace(s[i+1:])
	if rest == "" {
		return false
	}
	return rest[0] != 'Z'
}

// A stale tunnel state file may record a PID that the OS has since recycled
// for an unrelated process. StopTunnel must never signal that process.
func TestStopTunnelSkipsUnrelatedProcess(t *testing.T) {
	dir := withTempAXHome(t)

	// Unrelated long-lived process: plain sleep, cmdline has no kubectl marker.
	victim := exec.Command("sleep", "300")
	if err := victim.Start(); err != nil {
		t.Skipf("sleep unavailable: %v", err)
	}
	defer func() {
		_ = victim.Process.Kill()
		_ = victim.Wait()
	}()

	info := &TunnelInfo{Context: "stale-ctx", Port: 19999, PID: victim.Process.Pid}
	if err := SaveTunnel(info); err != nil {
		t.Fatalf("SaveTunnel: %v", err)
	}

	if err := StopTunnel(info); err != nil {
		t.Fatalf("StopTunnel: %v", err)
	}

	// Signal delivery is asynchronous; poll long enough to catch a kill.
	deadline := time.Now().Add(2 * time.Second)
	for processAlive(victim.Process.Pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !processAlive(victim.Process.Pid) {
		t.Fatalf("StopTunnel signaled an unrelated process (pid %d)", victim.Process.Pid)
	}
	if _, err := os.Stat(filepath.Join(dir, "tunnels", "stale-ctx.json")); !os.IsNotExist(err) {
		t.Fatalf("stale state file was not removed")
	}
}

// The genuine kubectl port-forward case must still be terminated.
func TestStopTunnelKillsPortForwardProcess(t *testing.T) {
	withTempAXHome(t)

	// Fake kubectl: long-lived sleep whose argv[0] carries the kubectl
	// port-forward signature, started in its own process group like the
	// real tunnel spawn (Setpgid). (A copied sleep binary run with
	// port-forward args would exit immediately on the invalid interval,
	// leaving a zombie — the kill assertion below would then pass
	// vacuously.)
	cmd := exec.Command("bash", "-c", "exec -a 'kubectl port-forward' sleep 300")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake kubectl: %v", err)
	}
	pid := cmd.Process.Pid

	info := &TunnelInfo{Context: "live-ctx", Port: 19998, PID: pid}
	if err := SaveTunnel(info); err != nil {
		t.Fatalf("SaveTunnel: %v", err)
	}

	if err := StopTunnel(info); err != nil {
		t.Fatalf("StopTunnel: %v", err)
	}

	// Reap and confirm it exited (SIGTERM was delivered).
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		t.Fatalf("port-forward process survived StopTunnel (pid %d)", pid)
	}
}
