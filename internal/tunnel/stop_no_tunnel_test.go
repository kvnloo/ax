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
	"strings"
	"testing"
)

// TestStopTunnelByContextNoTunnel: stopping a context with no state file
// surfaced a raw "open .../ctx.json: no such file or directory" read error.
// Map it to a friendly "no tunnel running" error instead. Red on base: the
// raw file error.
func TestStopTunnelByContextNoTunnel(t *testing.T) {
	axHome := t.TempDir()
	t.Setenv("AX_HOME", axHome)

	err := StopTunnelByContext("nosuchtunnel")
	if err == nil || !strings.Contains(err.Error(), "no tunnel running for context") {
		t.Fatalf("StopTunnelByContext err = %v, want no-tunnel-running error", err)
	}
}

// TestStopTunnelByContextCorruptFile: a state file with corrupt content is
// still a genuine error (not "no tunnel") — the mapping must only apply to
// a missing file.
func TestStopTunnelByContextCorruptFile(t *testing.T) {
	axHome := t.TempDir()
	t.Setenv("AX_HOME", axHome)

	dir, err := TunnelDir()
	if err != nil {
		t.Fatalf("TunnelDir: %v", err)
	}
	if err := os.WriteFile(dir+"/badctx.json", []byte("not json"), 0644); err != nil {
		t.Fatalf("writing corrupt state file: %v", err)
	}

	err = StopTunnelByContext("badctx")
	if err == nil || strings.Contains(err.Error(), "no tunnel running") {
		t.Fatalf("StopTunnelByContext err = %v, want the underlying parse error, not no-tunnel-running", err)
	}
}
