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
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TestProbeSeesFastExitWithoutEarlyReaper is the regression test for the
// detachReap/childExited ordering in spawnTunnel: the premature-exit probe
// must be able to observe a fast failure itself, which is only possible
// while no other reaper races it. Red on base (startPortForwardChild is
// new): with the old inline spawn there is no way to express "start the
// child but do not detach a reaper yet".
func TestProbeSeesFastExitWithoutEarlyReaper(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs waitpid")
	}
	logFile, err := os.CreateTemp(t.TempDir(), "child-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	cmd, err := startPortForwardChild("false", nil, logFile) // exits immediately
	if err != nil {
		t.Fatalf("starting child: %v", err)
	}
	// Deliberately no detachReap here: this is the probe window, where
	// childExited must observe the exit itself.

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if childExited(cmd.Process.Pid) {
			return // probe reaped and observed the fast exit
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("childExited never observed the fast exit: the probe is blind")
}

// TestDetachReapBlindsChildExited documents the premise behind the ordering
// in spawnTunnel: once a detached reaper has claimed the exit, childExited
// (WNOHANG waitpid) cannot observe it — the process is gone, ECHILD. So
// detachReap must not start until the premature-exit probe is finished.
// (Passes on base; it characterizes the defect, not the fix.)
func TestDetachReapBlindsChildExited(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs waitpid")
	}
	cmd, err := startPortForwardChild("false", nil, nil) // exits immediately
	if err != nil {
		t.Fatalf("starting child: %v", err)
	}
	detachReap(cmd)

	// Wait until somebody (us or the reaper) has claimed the exit.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var st syscall.WaitStatus
		wpid, werr := syscall.Wait4(cmd.Process.Pid, &st, syscall.WNOHANG, nil)
		if wpid == cmd.Process.Pid || (wpid == -1 && werr != nil) {
			break // exit claimed (by us) or already gone (by the reaper)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if childExited(cmd.Process.Pid) {
		t.Fatal("childExited observed an exit that was already reaped")
	}
}
