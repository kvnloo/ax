package tunnel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestListTunnelsIgnoresOrphanTmpFiles: SaveTunnel writes via a
// .tmp-*.json file renamed into place. A crash between CreateTemp and
// Rename leaves an orphan temp file holding a complete, valid TunnelInfo;
// it must not surface as a phantom tunnel.
func TestListTunnelsIgnoresOrphanTmpFiles(t *testing.T) {
	axHome := t.TempDir()
	t.Setenv("AX_HOME", axHome)

	info := &TunnelInfo{Context: "myctx", Namespace: "ax-system", Service: "ax-server", Port: 54321, PID: 1234}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshaling: %v", err)
	}
	dir := filepath.Join(axHome, "tunnels")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Orphan temp file with valid content, as left by a crashed SaveTunnel.
	if err := os.WriteFile(filepath.Join(dir, ".tmp-987654321.json"), data, 0644); err != nil {
		t.Fatalf("writing tmp file: %v", err)
	}
	// A real state file must still be listed.
	if err := os.WriteFile(filepath.Join(dir, "myctx.json"), data, 0644); err != nil {
		t.Fatalf("writing state file: %v", err)
	}

	tunnels, err := ListTunnels()
	if err != nil {
		t.Fatalf("ListTunnels: %v", err)
	}
	if len(tunnels) != 1 {
		t.Fatalf("got %d tunnels, want 1 (the orphan .tmp- file must be skipped)", len(tunnels))
	}
	if tunnels[0].Context != "myctx" || tunnels[0].Port != 54321 {
		t.Fatalf("wrong tunnel listed: %+v", tunnels[0])
	}
}
