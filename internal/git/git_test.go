package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danew/devsync/internal/apperrors"
)

func TestCompareLocalDetectsAheadBehindAndDivergence(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	run(t, repo, "git", "init")
	run(t, repo, "git", "config", "user.email", "devsync@example.com")
	run(t, repo, "git", "config", "user.name", "DevSync Test")
	writeFile(t, filepath.Join(repo, "file.txt"), "base")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "base")
	base := output(t, repo, "git", "rev-parse", "HEAD")

	writeFile(t, filepath.Join(repo, "file.txt"), "local")
	run(t, repo, "git", "commit", "-am", "local")
	local := output(t, repo, "git", "rev-parse", "HEAD")

	run(t, repo, "git", "checkout", "-b", "remote", base)
	writeFile(t, filepath.Join(repo, "file.txt"), "remote")
	run(t, repo, "git", "commit", "-am", "remote")
	remote := output(t, repo, "git", "rev-parse", "HEAD")

	comparison, err := compareLocal(ctx, repo, local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.LocalAhead != 1 || comparison.RemoteAhead != 1 {
		t.Fatalf("expected 1/1 divergence, got %d/%d", comparison.LocalAhead, comparison.RemoteAhead)
	}
}

func TestGitRemoteURLKeepsConfiguredPathInspectable(t *testing.T) {
	got := GitRemoteURL("core-dev", "~/workspace/work/steel-api")
	want := "core-dev:~/workspace/work/steel-api"
	if got != want {
		t.Fatalf("GitRemoteURL() = %q, want %q", got, want)
	}
}

func TestRemoteStateErrorDistinguishesMissingAndEmpty(t *testing.T) {
	missing := remoteStateError(RemoteMissing, "/missing")
	if !apperrors.Is(missing, apperrors.ErrRemoteRepoMissing) {
		t.Fatalf("expected missing error, got %v", missing)
	}
	empty := remoteStateError(RemoteEmptyDir, "/empty")
	if !apperrors.Is(empty, apperrors.ErrRemoteRepoMissing) {
		t.Fatalf("expected empty dir missing-class error, got %v", empty)
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func output(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %v failed: %v", name, args, err)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
