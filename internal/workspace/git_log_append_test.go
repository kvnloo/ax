package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// initLogRepo creates a git repo with a single marker file and returns its path.
func initLogRepo(t *testing.T, marker, content string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main", "-q")
	if err := os.WriteFile(filepath.Join(dir, marker), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "init")
	return dir
}

// The per-repo git diagnostics must keep every repo's section: with several
// repos, an operator debugging a broken workspace needs all of the errors,
// not just the last one.
func TestGitLogsKeepEveryRepo(t *testing.T) {
	origAXDir := AXDir
	AXDir = t.TempDir()
	defer func() { AXDir = origAXDir }()

	repo1 := initLogRepo(t, "one.txt", "repo1")
	repo2 := initLogRepo(t, "two.txt", "repo2")
	bad1 := filepath.Join(t.TempDir(), "no-such-repo-1")
	bad2 := filepath.Join(t.TempDir(), "no-such-repo-2")
	target := filepath.Join(t.TempDir(), "ws")

	repos := []*v1alpha1.GitRepo{
		{Repo: repo1, Dir: "code1", Branch: "main"},
		{Repo: bad1, Dir: "code2", Branch: "main"},
		{Repo: repo2, Dir: "code3", Branch: "main"},
		{Repo: bad2, Dir: "code4", Branch: "main"},
	}
	cloned, ok := cloneRepos(context.Background(), repos, target)

	if ok {
		t.Errorf("cloneRepos: ok = true, want false (two repos failed)")
	}
	if len(cloned) != 2 {
		t.Errorf("cloned = %v, want the two good repos", cloned)
	}

	errLog, err := os.ReadFile(filepath.Join(AXDir, gitErrorLog))
	if err != nil {
		t.Fatalf("reading %s: %v", gitErrorLog, err)
	}
	for _, bad := range []string{bad1, bad2} {
		if !strings.Contains(string(errLog), bad) {
			t.Errorf("%s missing section for %s", gitErrorLog, bad)
		}
	}

	okLog, err := os.ReadFile(filepath.Join(AXDir, gitSuccessLog))
	if err != nil {
		t.Fatalf("reading %s: %v", gitSuccessLog, err)
	}
	for _, good := range []string{repo1, repo2} {
		if !strings.Contains(string(okLog), good) {
			t.Errorf("%s missing section for %s", gitSuccessLog, good)
		}
	}
}
