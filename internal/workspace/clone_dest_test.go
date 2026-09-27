package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// initLocalRepo creates a git repo with a single marker file and returns its path.
func initLocalRepo(t *testing.T, marker, content string) string {
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

func TestCloneReposDuplicateDestinationSkipped(t *testing.T) {
	origAXDir := AXDir
	AXDir = t.TempDir()
	defer func() { AXDir = origAXDir }()

	repo1 := initLocalRepo(t, "one.txt", "repo1")
	repo2 := initLocalRepo(t, "two.txt", "repo2")
	target := filepath.Join(t.TempDir(), "ws")

	repos := []*v1alpha1.GitRepo{
		{Repo: repo1, Dir: "code", Branch: "main"},
		{Repo: repo2, Dir: "code", Branch: "main"},
	}
	cloned, ok := cloneRepos(context.Background(), repos, target)

	if !ok {
		t.Errorf("cloneRepos with duplicate destinations: ok = false, want true (first clone succeeded)")
	}
	if len(cloned) != 1 || cloned[0] != repo1 {
		t.Errorf("cloned = %v, want [%s]", cloned, repo1)
	}
	dest := filepath.Join(target, "code")
	if _, err := os.Stat(filepath.Join(dest, "one.txt")); err != nil {
		t.Errorf("first repo's checkout was replaced: one.txt missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "two.txt")); err == nil {
		t.Errorf("duplicate repo silently overwrote the first checkout: two.txt present")
	}
}

func TestCloneReposDuplicateDerivedNameSkipped(t *testing.T) {
	origAXDir := AXDir
	AXDir = t.TempDir()
	defer func() { AXDir = origAXDir }()

	// Different hosts, same derived directory name.
	repo1 := initLocalRepo(t, "one.txt", "repo1")
	repo2 := initLocalRepo(t, "two.txt", "repo2")
	target := filepath.Join(t.TempDir(), "ws")

	repos := []*v1alpha1.GitRepo{
		{Repo: repo1, Name: "shared", Branch: "main"},
		{Repo: repo2, Name: "shared", Branch: "main"},
	}
	cloned, _ := cloneRepos(context.Background(), repos, target)

	if len(cloned) != 1 || cloned[0] != repo1 {
		t.Errorf("cloned = %v, want [%s]", cloned, repo1)
	}
	if _, err := os.Stat(filepath.Join(target, "shared", "two.txt")); err == nil {
		t.Errorf("duplicate derived name silently overwrote the first checkout")
	}
}

func TestCloneReposDistinctDestinationsBothCloned(t *testing.T) {
	repo1 := initLocalRepo(t, "one.txt", "repo1")
	repo2 := initLocalRepo(t, "two.txt", "repo2")
	target := filepath.Join(t.TempDir(), "ws")

	repos := []*v1alpha1.GitRepo{
		{Repo: repo1, Dir: "code1", Branch: "main"},
		{Repo: repo2, Dir: "code2", Branch: "main"},
	}
	cloned, ok := cloneRepos(context.Background(), repos, target)

	if !ok {
		t.Errorf("cloneRepos distinct destinations: ok = false, want true")
	}
	if len(cloned) != 2 {
		t.Errorf("cloned = %v, want both repos", cloned)
	}
}
