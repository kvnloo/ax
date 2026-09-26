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
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TestSignalZeroCannotSeeZombieChild documents why spawnTunnel's
// premature-exit probe cannot use signal 0: an exited-but-unreaped child of
// this process is a zombie and still answers signal 0. (Passes on base; it
// characterizes the defect, not the fix.)
func TestSignalZeroCannotSeeZombieChild(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs unix process semantics")
	}
	cmd := exec.Command("false") // exits immediately
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(500 * time.Millisecond) // let it exit; deliberately no Wait
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		t.Fatalf("expected signal 0 to succeed on our zombie child, got %v", err)
	}
	_ = cmd.Wait() // clean up
}

// TestChildExitedDetectsZombieChild is the regression test for spawnTunnel's
// premature-exit probe: the old signal-0 check was blind to fast kubectl
// failures (zombies answer signal 0), so they fell through to the 5s timeout
// path. The zombie-aware probe must detect the exit promptly, and the
// WNOHANG waitpid must leave no zombie behind. Red on base (childExited is
// new).
func TestChildExitedDetectsZombieChild(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs waitpid")
	}
	cmd := exec.Command("false") // exits immediately
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if childExited(pid) {
			if st := procState(t, pid); st != "" {
				t.Fatalf("childExited detected the exit but left a /proc entry (state %q)", st)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = cmd.Wait()
	t.Fatal("childExited did not detect the exited child within 2s")
}

// TestChildExitedFalseForLiveChild guards the other direction: a running
// child must never be reported exited (that would abort a healthy spawn).
func TestChildExitedFalseForLiveChild(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs waitpid")
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if childExited(cmd.Process.Pid) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("childExited reported a live child as exited")
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}
