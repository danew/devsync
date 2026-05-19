package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danew/devsync/internal/apperrors"
	devssh "github.com/danew/devsync/internal/ssh"
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

func TestCaptureOriginReturnsNilWhenOriginMissing(t *testing.T) {
	ctx := context.Background()
	repo := initializedRepo(t)

	origin, err := CaptureOrigin(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if origin != nil {
		t.Fatalf("expected no origin, got %#v", origin)
	}
}

func TestRestoreRemoteOriginReplacesBootstrapMirrorAndAllowsPull(t *testing.T) {
	ctx := context.Background()
	runner := localSSHRunner(t)
	local := initializedRepo(t)
	upstream := filepath.Join(t.TempDir(), "upstream.git")
	run(t, "", "git", "init", "--bare", upstream)
	run(t, local, "git", "remote", "add", "origin", upstream)
	run(t, local, "git", "push", "-u", "origin", "main")

	origin, err := CaptureOrigin(ctx, local)
	if err != nil {
		t.Fatal(err)
	}
	remoteClone, bootstrapMirror := bootstrapClone(t, local)
	os.RemoveAll(bootstrapMirror)

	remoteList, err := RestoreRemoteOrigin(ctx, runner, remoteClone, origin, bootstrapMirror)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(remoteList, bootstrapMirror) {
		t.Fatalf("bootstrap mirror leaked into remote topology:\n%s", remoteList)
	}
	if !strings.Contains(remoteList, upstream) {
		t.Fatalf("canonical origin missing from remote topology:\n%s", remoteList)
	}
	run(t, remoteClone, "git", "pull")
}

func TestRestoreRemoteOriginRemovesTemporaryOriginWhenLocalOriginMissing(t *testing.T) {
	ctx := context.Background()
	runner := localSSHRunner(t)
	local := initializedRepo(t)
	remoteClone, bootstrapMirror := bootstrapClone(t, local)
	os.RemoveAll(bootstrapMirror)

	remoteList, err := RestoreRemoteOrigin(ctx, runner, remoteClone, nil, bootstrapMirror)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(remoteList) != "" {
		t.Fatalf("expected no remotes after missing-origin bootstrap, got:\n%s", remoteList)
	}
}

func TestCaptureOriginIgnoresAdditionalRemotes(t *testing.T) {
	ctx := context.Background()
	local := initializedRepo(t)
	originPath := filepath.Join(t.TempDir(), "origin.git")
	backupPath := filepath.Join(t.TempDir(), "backup.git")
	run(t, "", "git", "init", "--bare", originPath)
	run(t, "", "git", "init", "--bare", backupPath)
	run(t, local, "git", "remote", "add", "origin", originPath)
	run(t, local, "git", "remote", "add", "backup", backupPath)

	origin, err := CaptureOrigin(ctx, local)
	if err != nil {
		t.Fatal(err)
	}
	if origin == nil || origin.Name != "origin" || origin.FetchURL != originPath {
		t.Fatalf("expected canonical origin %q, got %#v", originPath, origin)
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

func initializedRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run(t, repo, "git", "init")
	run(t, repo, "git", "checkout", "-b", "main")
	run(t, repo, "git", "config", "user.email", "devsync@example.com")
	run(t, repo, "git", "config", "user.name", "DevSync Test")
	writeFile(t, filepath.Join(repo, "file.txt"), "base")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "base")
	return repo
}

func bootstrapClone(t *testing.T, local string) (string, string) {
	t.Helper()
	root := t.TempDir()
	mirror := filepath.Join(root, "repo.mirror.git")
	remoteClone := filepath.Join(root, "remote")
	run(t, local, "git", "clone", "--mirror", ".", mirror)
	run(t, "", "git", "clone", mirror, remoteClone)
	return remoteClone, mirror
}

func localSSHRunner(t *testing.T) devssh.Runner {
	t.Helper()
	bin := t.TempDir()
	ssh := filepath.Join(bin, "ssh")
	script := "#!/bin/sh\nif [ \"$1\" = \"-p\" ]; then shift 2; fi\nshift\nexec sh -c \"$1\"\n"
	if err := os.WriteFile(ssh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return devssh.Runner{Target: devssh.Target{Host: "local-test"}}
}
