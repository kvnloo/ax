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

package runner_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/ax/internal/workspace"
	"github.com/google/ax/pkg/apis/v1alpha1"
)

// The runner's contract: "Workspace setup failures are logged but do not
// abort the run; the task simply never reports ready." A failed git clone is a
// setup failure, so /readyz must stay 503 even though the runner keeps going.
func TestRun_GitFailureNeverReportsReady(t *testing.T) {
	h := newHarness(t)
	h.cfg.Workspaces = []*v1alpha1.Workspace{
		{
			Metadata: &v1alpha1.ObjectMeta{Name: "ws"},
			Spec: &v1alpha1.WorkspaceSpec{
				Git: []*v1alpha1.GitRepo{
					{Name: "bad", Repo: "https://127.0.0.1:9/nonexistent/repo.git"},
				},
			},
		},
	}
	h.start(t)

	// Wait for setup to finish. The failed clone records git-error.log under
	// AXDir (overridden by the harness); only then is the ready verdict final.
	// (The metadata server answers 503 while setup is still running, so
	// checking /readyz before this point would prove nothing.)
	errLog := filepath.Join(workspace.AXDir, "git-error.log")
	deadline := time.Now().Add(60 * time.Second)
	for {
		if _, err := os.Stat(errLog); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("git-error.log never appeared; setup may be stuck")
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Setup is done and the clone failed: /readyz must never report ready.
	readyURL := fmt.Sprintf("http://127.0.0.1:%d/readyz", h.cfg.Port)
	deadline = time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get(readyURL)
		if err != nil {
			t.Fatalf("readyz not reachable after setup finished: %v", err)
		}
		status := resp.StatusCode
		resp.Body.Close()
		if status == http.StatusOK {
			t.Fatal("readyz reported 200 despite git clone failure; want 503")
		}
		if status != http.StatusServiceUnavailable {
			t.Fatalf("readyz = %d after failed git setup; want 503", status)
		}
		if time.Now().After(deadline) {
			return // stayed 503 throughout: not ready, as required
		}
		time.Sleep(100 * time.Millisecond)
	}
}
