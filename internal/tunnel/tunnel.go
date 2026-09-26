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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var portForwardRegex = regexp.MustCompile(`Forwarding from .*:(\d+)\s+->`)

type TunnelInfo struct {
	Context   string    `json:"context"`
	Namespace string    `json:"namespace"`
	Service   string    `json:"service"`
	Port      int       `json:"port"`
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
}

type Options struct {
	ServerURL string
	Context   string
	Namespace string // default: ax-system
	Service   string // default: ax-server
	Target    string // e.g. "pod/name" or "svc/name" (overrides Service if set)
	Port      int    // remote port, default: 8080
}

func SanitizeContext(name string) string {
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	s := sb.String()
	if s == "" {
		return "default"
	}
	return s
}

func TunnelDir() (string, error) {
	if dir := os.Getenv("AX_HOME"); dir != "" {
		p := filepath.Join(dir, "tunnels")
		return p, os.MkdirAll(p, 0755)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(home, ".ax", "tunnels")
	return p, os.MkdirAll(p, 0755)
}

func GetTunnel(ctxName string) (*TunnelInfo, error) {
	dir, err := TunnelDir()
	if err != nil {
		return nil, err
	}
	statePath := filepath.Join(dir, SanitizeContext(ctxName)+".json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}
	var info TunnelInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func SaveTunnel(info *TunnelInfo) error {
	dir, err := TunnelDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	// Write atomically: a crash mid-write must never leave a torn state
	// file behind. A torn file makes GetTunnel fail, so ctx show reports
	// "no tunnel running" while the port-forward is still alive and the
	// next command auto-connects a second tunnel, orphaning the first.
	// Rename within the same directory is atomic on POSIX: after a crash
	// the path holds either the old or the new complete file, never a
	// partial one.
	statePath := filepath.Join(dir, SanitizeContext(info.Context)+".json")
	tmp, err := os.CreateTemp(dir, ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, statePath)
}

func StopTunnel(info *TunnelInfo) error {
	if info == nil {
		return nil
	}
	if isPortForwardProcess(info.PID) {
		// Attempt to terminate process group and single process
		_ = syscall.Kill(-info.PID, syscall.SIGTERM)
		_ = syscall.Kill(info.PID, syscall.SIGTERM)
	}
	// The state file is removed regardless: if the process is gone or is not
	// ours, the entry is stale either way.

	dir, err := TunnelDir()
	if err == nil {
		_ = os.Remove(filepath.Join(dir, SanitizeContext(info.Context)+".json"))
	}
	return nil
}

// isPortForwardProcess reports whether pid is a live process that looks like a
// kubectl port-forward started by this tool. Tunnel state files can outlive
// their process, and the recorded PID may since have been recycled by the OS
// for an unrelated process, which must never be signaled.
func isPortForwardProcess(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return false // not alive (or not signalable)
	}
	// On Linux, verify the command line before signaling. Where /proc is
	// unavailable (other platforms) fall back to the liveness check above so
	// behavior there is unchanged.
	//
	// During execve the kernel may briefly report an empty cmdline for a
	// live process; poll briefly so a just-(re)exec'd port-forward is not
	// misread as foreign. A zombie's cmdline stays empty and correctly
	// reads as not-ours after the retries.
	for i := 0; i < 5; i++ {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			return true
		}
		if len(data) > 0 {
			cmd := string(data)
			return strings.Contains(cmd, "kubectl") && strings.Contains(cmd, "port-forward")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func StopTunnelByContext(ctxName string) error {
	dir, err := TunnelDir()
	if err != nil {
		return err
	}
	// Take the spawn lock so a concurrent EnsureServerURL cannot save a new
	// tunnel entry between our Get and our Stop.
	return withTunnelLock(dir, func() error {
		info, err := GetTunnel(ctxName)
		if err != nil {
			return err
		}
		return StopTunnel(info)
	})
}

// withTunnelLock runs fn while holding an exclusive advisory lock on the
// tunnel state directory, so concurrent ax processes cannot interleave the
// check-then-spawn sequence in EnsureServerURL. Without it, two processes
// starting at once both find no active tunnel, both spawn a kubectl
// port-forward, and both SaveTunnel; the last write wins and the other
// tunnel is orphaned — no state file, still holding its local port.
//
// The lock is a ".lock" file in the tunnel directory held via flock(LOCK_EX)
// for the duration of fn; a crashed holder releases it when its file
// descriptor closes. Callers must not nest withTunnelLock: flock locks are
// per open file description, so re-entering from the same process
// deadlocks.
func withTunnelLock(dir string, fn func() error) error {
	lockPath := filepath.Join(dir, ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("opening tunnel lock file: %w", err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquiring tunnel lock: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

func ListTunnels() ([]*TunnelInfo, error) {
	dir, err := TunnelDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var results []*TunnelInfo
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		// Skip SaveTunnel's temp files: a crash between CreateTemp and
		// Rename leaves an orphan .tmp-*.json behind, and one written after
		// the payload was flushed holds a complete, valid TunnelInfo. It
		// must never surface as a phantom tunnel in `ax tunnel list`.
		if strings.HasPrefix(name, ".tmp-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var info TunnelInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}
		results = append(results, &info)
	}
	return results, nil
}

func IsTunnelHealthy(port int) bool {
	if port <= 0 {
		return false
	}
	client := http.Client{Timeout: 400 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// IsTunnelActive reports whether the tunnel described by info is genuinely
// ours and responding: the recorded process must still be the kubectl
// port-forward that created it (see isPortForwardProcess) and the local
// health check must pass. A 200 from /healthz alone is not sufficient — a
// state file can outlive its process while the OS recycles the port for an
// unrelated local service, which would otherwise display as Active.
func IsTunnelActive(info *TunnelInfo) bool {
	if info == nil {
		return false
	}
	return isPortForwardProcess(info.PID) && IsTunnelHealthy(info.Port)
}

// EnsureServerURL resolves the AX server URL.
// If ServerURL or AX_SERVER is explicitly set, it returns that.
// Otherwise, it checks the active Kubernetes context (or opts.Context)
// and establishes/reuses a background port-forward tunnel to svc/ax-server.
func EnsureServerURL(opts Options) (string, error) {
	if opts.ServerURL != "" {
		return opts.ServerURL, nil
	}
	if env := os.Getenv("AX_SERVER"); env != "" {
		return env, nil
	}

	ctxName, err := CurrentContext(opts.Context)
	if err != nil || ctxName == "" {
		// No Kubernetes context found; fallback to local default
		return "http://localhost:8080", nil
	}

	if opts.Namespace == "" {
		if envNs := os.Getenv("AX_SYSTEM_NAMESPACE"); envNs != "" {
			opts.Namespace = envNs
		} else {
			opts.Namespace = "ax-system"
		}
	}
	if opts.Service == "" {
		opts.Service = "ax-server"
	}
	if opts.Port == 0 {
		opts.Port = 8080
	}

	dir, err := TunnelDir()
	if err != nil {
		return "", fmt.Errorf("creating tunnel directory: %w", err)
	}

	// Serialize the check-then-spawn sequence below across concurrent ax
	// processes; see withTunnelLock.
	serverURL := ""
	if err := withTunnelLock(dir, func() error {
		// Check for existing active tunnel. The recorded process must still be
		// the kubectl port-forward that created the entry: a stale state file
		// with a recycled port serving an unrelated /healthz 200 must be reaped,
		// not adopted (see IsTunnelActive).
		if existing, err := GetTunnel(ctxName); err == nil && existing != nil {
			if IsTunnelActive(existing) {
				serverURL = fmt.Sprintf("http://127.0.0.1:%d", existing.Port)
				return nil
			}
			// Existing tunnel is unhealthy or stale, clean it up
			_ = StopTunnel(existing)
		}

		// Start new background port-forward for this context
		url, err := spawnTunnel(ctxName, dir, opts)
		if err != nil {
			return err
		}
		serverURL = url
		return nil
	}); err != nil {
		return "", err
	}
	return serverURL, nil
}

// killAndReapChild kills cmd's process (best effort) and reaps it. An
// unreaped child stays a zombie in this process until the CLI exits, which
// matters for long-lived sessions (interactive ssh, watch) that spawn and
// discard port-forwards repeatedly.
func killAndReapChild(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

// detachReap starts a goroutine that reaps cmd when it exits. Use for
// intentionally long-lived children: the tunnel port-forward outlives
// spawnTunnel, and StopTunnel can only signal it by PID (never Wait on it),
// so without this the child would linger as a zombie for the rest of the
// CLI's lifetime after being stopped.
func detachReap(cmd *exec.Cmd) {
	go func() { _ = cmd.Wait() }()
}

// childExited reports whether pid — a child of this process started via
// os/exec — has already exited. A non-blocking waitpid both detects and
// reaps the exit, so the child never lingers as a zombie. Signal 0 is not a
// substitute: an unreaped child is a zombie and still answers signal 0,
// which made spawnTunnel's premature-exit probe blind to fast kubectl
// failures (they fell through to the 5s timeout path instead of failing
// fast with kubectl's own error output).
//
// Note: the WNOHANG waitpid reaps the child, so a later cmd.Wait on the same
// *exec.Cmd returns an error; callers must ignore it.
func childExited(pid int) bool {
	var status syscall.WaitStatus
	wpid, err := syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
	return err == nil && wpid == pid
}

// startPortForwardChild starts a child process writing its output to logFile
// (nil discards it). Reaping stays the caller's job: callers that need the
// premature-exit probe must NOT hand the child to detachReap until the probe
// is done — a detached reaper racing childExited claims fast exits first and
// blinds the probe (WNOHANG waitpid can only observe an unreaped child).
func startPortForwardChild(name string, args []string, logFile *os.File) (*exec.Cmd, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// spawnTunnel starts a background kubectl port-forward for ctxName, waits for
// the local port assignment and a passing health check, records the tunnel
// state, and returns the server URL. Callers must hold the tunnel lock (see
// withTunnelLock) so two processes cannot spawn for the same context at once.
func spawnTunnel(ctxName, dir string, opts Options) (string, error) {
	logPath := filepath.Join(dir, SanitizeContext(ctxName)+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return "", fmt.Errorf("opening tunnel log: %w", err)
	}
	defer logFile.Close()

	var kArgs []string
	if ctxName != "" {
		kArgs = append(kArgs, "--context", ctxName)
	}
	targetRes := fmt.Sprintf("svc/%s", opts.Service)
	if opts.Target != "" {
		targetRes = opts.Target
	}
	kArgs = append(kArgs, "port-forward", "-n", opts.Namespace, targetRes, fmt.Sprintf(":%d", opts.Port))

	cmd, err := startPortForwardChild("kubectl", kArgs, logFile)
	if err != nil {
		return "", fmt.Errorf("spawning kubectl port-forward for context %q: %w", ctxName, err)
	}
	// NOTE: detachReap is intentionally NOT started here. The premature-exit
	// probe below relies on childExited's WNOHANG waitpid; a detached reaper
	// racing it would reap fast kubectl failures first and blind the probe,
	// sending real errors down the 5s timeout path instead of failing fast.
	// It starts only after the tunnel is healthy (below); every failure path
	// between here and there reaps explicitly — via childExited itself on the
	// fast-failure path, via killAndReapChild on the timeout/health paths.

	// Wait for port assignment from log output
	localPort := 0
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)

		logData, err := os.ReadFile(logPath)
		if err != nil {
			continue
		}
		if match := portForwardRegex.FindStringSubmatch(string(logData)); len(match) > 1 {
			p, err := strconv.Atoi(match[1])
			if err == nil && p > 0 {
				localPort = p
				break
			}
		}

		// Check if command exited prematurely. Signal 0 cannot tell: our own
		// unreaped child is a zombie and still answers signal 0, so use the
		// zombie-aware probe. childExited reaps the exit via WNOHANG, so no
		// further cleanup of the child is needed on this path.
		if childExited(cmd.Process.Pid) {
			return "", fmt.Errorf("kubectl port-forward exited for context %q: %s", ctxName, strings.TrimSpace(string(logData)))
		}
	}

	if localPort == 0 {
		killAndReapChild(cmd)
		logData, _ := os.ReadFile(logPath)
		return "", fmt.Errorf("timeout waiting for kubectl port-forward on context %q: %s", ctxName, strings.TrimSpace(string(logData)))
	}

	// Wait for healthz to respond
	healthy := false
	healthDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(healthDeadline) {
		if IsTunnelHealthy(localPort) {
			healthy = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !healthy {
		killAndReapChild(cmd)
		return "", fmt.Errorf("tunnel started on port %d but health check failed for context %q", localPort, ctxName)
	}

	// The tunnel is healthy: the port-forward is meant to outlive this
	// function, and StopTunnel can only signal it by PID, so hand it to the
	// detached reaper now that the premature-exit probe above is finished.
	// Without this Wait the child would linger as a zombie for the rest of
	// the CLI's lifetime after being stopped.
	detachReap(cmd)

	info := &TunnelInfo{
		Context:   ctxName,
		Namespace: opts.Namespace,
		Service:   opts.Service,
		Port:      localPort,
		PID:       cmd.Process.Pid,
		CreatedAt: time.Now(),
	}
	if err := recordTunnelOrCleanup(info, cmd); err != nil {
		return "", err
	}

	return fmt.Sprintf("http://127.0.0.1:%d", localPort), nil
}

// recordTunnelOrCleanup persists the tunnel state. If recording fails the
// tunnel is torn down instead of being left running with no state file: an
// untracked port-forward is invisible to `ax tunnel list`, unreachable by
// `ax tunnel stop`, and the next command spawns a second tunnel on top of
// it — the orphan scenario atomic SaveTunnel guards against.
func recordTunnelOrCleanup(info *TunnelInfo, cmd *exec.Cmd) error {
	if err := SaveTunnel(info); err != nil {
		killAndReapChild(cmd)
		return fmt.Errorf("recording tunnel state: %w", err)
	}
	return nil
}

// PortForward starts an ephemeral port-forward to a Kubernetes resource (e.g. pod/name or svc/name)
// and returns the local assigned port and a cleanup function.
func PortForward(ctx context.Context, kubeContext, namespace, targetResource string, remotePort int) (int, func(), error) {
	ctxName, err := CurrentContext(kubeContext)
	if err != nil {
		ctxName = ""
	}

	var kArgs []string
	if ctxName != "" {
		kArgs = append(kArgs, "--context", ctxName)
	}
	if namespace != "" {
		kArgs = append(kArgs, "-n", namespace)
	}
	kArgs = append(kArgs, "port-forward", targetResource, fmt.Sprintf(":%d", remotePort))

	cmd := exec.CommandContext(ctx, "kubectl", kArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	pr, pw, err := os.Pipe()
	if err != nil {
		return 0, nil, fmt.Errorf("creating pipe: %w", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		return 0, nil, fmt.Errorf("spawning kubectl port-forward for %s: %w", targetResource, err)
	}

	portChan := make(chan int, 1)
	errChan := make(chan error, 1)

	go scanPortForwardOutput(pr, portChan, errChan)

	cleanup := func() {
		_ = pr.Close()
		_ = pw.Close()
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			_ = cmd.Process.Kill()
			// Reap the child: without Wait it would linger as a zombie
			// until the CLI exits.
			_ = cmd.Wait()
		}
	}

	select {
	case p := <-portChan:
		return p, cleanup, nil
	case err := <-errChan:
		cleanup()
		return 0, nil, err
	case <-time.After(8 * time.Second):
		cleanup()
		return 0, nil, fmt.Errorf("timed out waiting for port-forward to %s", targetResource)
	case <-ctx.Done():
		cleanup()
		return 0, nil, ctx.Err()
	}
}

// scanPortForwardOutput watches kubectl port-forward's stdout for the local
// port assignment line and reports it on portChan. Only the FIRST match is
// delivered: kubectl reprints the forwarding line (and "Handling connection"
// lines keep the accumulated buffer matching), and portChan carries a single
// buffered slot consumed exactly once by PortForward — resending would block
// the watcher goroutine forever on a channel nobody will read again,
// leaking the goroutine for the life of the process.
func scanPortForwardOutput(r io.Reader, portChan chan<- int, errChan chan<- error) {
	buf := make([]byte, 1024)
	var output strings.Builder
	sent := false
	for {
		n, err := r.Read(buf)
		if n > 0 {
			output.Write(buf[:n])
			if !sent {
				if match := portForwardRegex.FindStringSubmatch(output.String()); len(match) > 1 {
					if p, convErr := strconv.Atoi(match[1]); convErr == nil && p > 0 {
						sent = true
						portChan <- p
					}
				}
			}
		}
		if err != nil {
			select {
			case errChan <- fmt.Errorf("kubectl port-forward exited: %s", strings.TrimSpace(output.String())):
			default:
			}
			return
		}
	}
}
