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

package workspace_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/ax/internal/workspace"
	"github.com/google/ax/pkg/apis/v1alpha1"
)

// A failed git clone must surface as an error. The runner's documented
// contract is that workspace setup failures never report ready, and it can
// only honor that if the failure is visible to it.
func TestSetupWorkspace_GitFailureReturnsError(t *testing.T) {
	tempDir := t.TempDir()

	ws := &v1alpha1.Workspace{
		Metadata: &v1alpha1.ObjectMeta{
			Name: "test-git-fail-ws",
		},
		Spec: &v1alpha1.WorkspaceSpec{
			Git: []*v1alpha1.GitRepo{
				{
					Name: "invalid-repo",
					Repo: "https://127.0.0.1:9/nonexistent/repo.git",
				},
			},
		},
	}

	origAXDir := workspace.AXDir
	workspace.AXDir = filepath.Join(tempDir, "ax-state")
	defer func() { workspace.AXDir = origAXDir }()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if _, err := workspace.SetupWorkspace(ctx, ws, tempDir, ""); err == nil {
		t.Fatal("expected SetupWorkspace to return an error when git clone fails, got nil")
	}
}
