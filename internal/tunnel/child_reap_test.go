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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// procState returns the single-letter process state from /proc/<pid>/stat,
// or "" when the process no longer exists (fully reaped).
func procState(t *testing.T, pid int) string {
	t.Helper()
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	// comm is parenthesized and may itself contain spaces or ')'; the state
	// is the first field after the final ')'.
	rest := data[bytes.LastIndexByte(data, ')')+1:]
	fields := strings.Fields(string(rest))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// TestUnreapedChildBecomesZombie documents the premise both fixes rely on:
// Go does not reap child processes for you, so a started-but-never-Waited
// child sits in /proc as state Z until the process exits. (Passes on base;
// it characterizes the bug, not the fix.)
func TestUnreapedChildBecomesZombie(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs /proc")
	}
	cmd := exec.Command("sleep", "0.05")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	time.Sleep(500 * time.Millisecond) // let it exit; deliberately no Wait
	if st := procState(t, pid); st != "Z" {
		t.Fatalf("expected zombie (state Z) for unreaped child, got %q", st)
	}
	_ = cmd.Wait() // clean up
}

// TestKillAndReapChildLeavesNoZombie is the regression test for the
// spawnTunnel failure paths: Kill alone left a zombie; killAndReapChild
// must leave no /proc entry at all. Red on base (killAndReapChild is new).
func TestKillAndReapChildLeavesNoZombie(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs /proc")
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	killAndReapChild(cmd)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st := procState(t, pid); st == "" {
			return // fully reaped
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child %d still present after killAndReapChild (state %q)", pid, procState(t, pid))
}

// TestDetachReapReapsExitedDaemon is the regression test for the spawnTunnel
// success path: the port-forward outlives spawnTunnel, so its reap happens
// in a detached goroutine once the child exits. Red on base (detachReap is
// new).
func TestDetachReapReapsExitedDaemon(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs /proc")
	}
	cmd := exec.Command("sleep", "0.05")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	detachReap(cmd)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st := procState(t, pid); st == "" {
			return // reaped
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child %d still present after detachReap (state %q)", pid, procState(t, pid))
}
