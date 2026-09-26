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
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestTunnelLockSerializesHolders proves the mutual-exclusion property that
// the EnsureServerURL check-then-spawn sequence relies on: N concurrent
// holders of withTunnelLock never overlap inside the critical section.
func TestTunnelLockSerializesHolders(t *testing.T) {
	dir := t.TempDir()

	var current, maxSeen atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := withTunnelLock(dir, func() error {
				n := current.Add(1)
				for {
					m := maxSeen.Load()
					if n <= m || maxSeen.CompareAndSwap(m, n) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				current.Add(-1)
				return nil
			}); err != nil {
				t.Errorf("withTunnelLock: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := maxSeen.Load(); got != 1 {
		t.Fatalf("critical sections overlapped: max concurrent holders = %d, want 1", got)
	}
}

// TestTunnelLockSurvivesCrashedHolder documents the crash-safety property the
// orphaned-tunnel fix depends on: flock releases when the holder's file
// descriptor closes (as on process death), so a crashed spawn never wedges
// later ax invocations behind a stale lock.
func TestTunnelLockSurvivesCrashedHolder(t *testing.T) {
	dir := t.TempDir()

	// Simulate a holder that "crashes": acquire the raw flock, then close the
	// fd without unlocking — the kernel must release the lock on close.
	lockPath := filepath.Join(dir, ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	_ = f.Close() // crash: no unlock

	done := make(chan error, 1)
	go func() {
		done <- withTunnelLock(dir, func() error { return nil })
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("lock not released after holder crash: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("withTunnelLock blocked forever behind a crashed holder")
	}
}
