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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestRecordTunnelOrCleanupTearsDownOnSaveFailure is the regression test for
// spawnTunnel ignoring SaveTunnel errors: a healthy port-forward with no
// state file is an untracked orphan — invisible to `ax tunnel list`,
// unreachable by `ax tunnel stop`, and the next command spawns a second
// tunnel on top of it. On a record failure the child must be torn down and
// the error surfaced. Red on base (recordTunnelOrCleanup is new).
func TestRecordTunnelOrCleanupTearsDownOnSaveFailure(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs waitpid")
	}
	// AX_HOME pointing at a regular file makes TunnelDir's MkdirAll fail,
	// so SaveTunnel cannot record.
	blocker, err := os.CreateTemp(t.TempDir(), "axhome-blocker")
	if err != nil {
		t.Fatal(err)
	}
	blocker.Close()
	t.Setenv("AX_HOME", blocker.Name())

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting child: %v", err)
	}

	info := &TunnelInfo{Context: "record-fail-ctx", Port: 19991, PID: cmd.Process.Pid}
	if err := recordTunnelOrCleanup(info, cmd); err == nil {
		t.Fatal("expected an error when the tunnel state cannot be recorded")
	}
	// The child must be gone, not an untracked orphan: killAndReapChild is
	// synchronous, so the exit is already reaped and childExited sees ECHILD.
	if childExited(cmd.Process.Pid) {
		t.Fatal("child still alive after failed state record: untracked orphan")
	}
}

// TestRecordTunnelOrCleanupKeepsHealthyTunnel verifies the happy path: a
// recorded tunnel is left running and the state file exists.
func TestRecordTunnelOrCleanupKeepsHealthyTunnel(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs waitpid")
	}
	home := t.TempDir()
	t.Setenv("AX_HOME", home)

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting child: %v", err)
	}
	defer killAndReapChild(cmd)

	info := &TunnelInfo{Context: "record-ok-ctx", Port: 19992, PID: cmd.Process.Pid}
	if err := recordTunnelOrCleanup(info, cmd); err != nil {
		t.Fatalf("recording tunnel state: %v", err)
	}
	statePath := filepath.Join(home, "tunnels", "record-ok-ctx.json")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}
	if childExited(cmd.Process.Pid) {
		t.Fatal("healthy recorded tunnel was killed")
	}
}
