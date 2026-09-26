package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A torn state file (crash mid-write) makes GetTunnel fail and ListTunnels
// silently drop the entry, so ctx show reports "no tunnel running" while the
// port-forward is still alive. Documents the failure mode SaveTunnel's
// atomic write exists to prevent.
func TestTornStateFileBreaksGetTunnel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AX_HOME", dir)

	info := &TunnelInfo{Context: "torn", Namespace: "ns", Service: "svc", Port: 18081, PID: 1234, CreatedAt: time.Now()}
	if err := SaveTunnel(info); err != nil {
		t.Fatalf("SaveTunnel: %v", err)
	}
	statePath := filepath.Join(dir, "tunnels", "torn.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Simulate a crash mid-write: truncate the file.
	if err := os.WriteFile(statePath, data[:len(data)/2], 0644); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := GetTunnel("torn"); err == nil {
		t.Fatal("expected GetTunnel to fail on a torn state file, got nil")
	}
	found := false
	for _, ti := range mustListTunnels(t) {
		if ti.Context == "torn" {
			found = true
		}
	}
	if found {
		t.Fatal("ListTunnels should skip the torn entry")
	}
}

// SaveTunnel must round-trip through GetTunnel and leave no temp files
// behind: the atomic rename is the whole point, so assert the directory
// contains exactly the state file afterwards.
func TestSaveTunnelAtomicRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AX_HOME", dir)

	info := &TunnelInfo{Context: "atomic", Namespace: "ns", Service: "svc", Port: 18082, PID: 5678, CreatedAt: time.Now().Truncate(time.Second)}
	if err := SaveTunnel(info); err != nil {
		t.Fatalf("SaveTunnel: %v", err)
	}
	got, err := GetTunnel("atomic")
	if err != nil {
		t.Fatalf("GetTunnel: %v", err)
	}
	if got.Port != info.Port || got.PID != info.PID || got.Context != info.Context {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "tunnels"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "atomic.json" {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected only atomic.json in tunnel dir, got %v", names)
	}
	// Overwriting must also be atomic and clean.
	info2 := &TunnelInfo{Context: "atomic", Namespace: "ns", Service: "svc", Port: 18083, PID: 9999, CreatedAt: time.Now().Truncate(time.Second)}
	if err := SaveTunnel(info2); err != nil {
		t.Fatalf("SaveTunnel overwrite: %v", err)
	}
	got2, err := GetTunnel("atomic")
	if err != nil {
		t.Fatalf("GetTunnel after overwrite: %v", err)
	}
	if got2.Port != 18083 {
		t.Fatalf("overwrite did not take: %+v", got2)
	}
	entries, _ = os.ReadDir(filepath.Join(dir, "tunnels"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("temp file leaked: %s", e.Name())
		}
	}
}

func mustListTunnels(t *testing.T) []*TunnelInfo {
	t.Helper()
	ts, err := ListTunnels()
	if err != nil {
		t.Fatalf("ListTunnels: %v", err)
	}
	return ts
}
