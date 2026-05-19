package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/danew/devsync/internal/apperrors"
)

func TestResolveConfigUsesConventionWhenConfigsAreAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "remote", "work", "nested", "example")
	mustMkdir(t, repo)

	cfg, err := ResolveConfig(Workspace{Name: "example", Root: repo})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Workspace.Name != "example" {
		t.Fatalf("workspace name = %q", cfg.Workspace.Name)
	}
	if cfg.Remote.Node != "core-dev" || cfg.Remote.Host != "core-dev" {
		t.Fatalf("remote node/host = %q/%q", cfg.Remote.Node, cfg.Remote.Host)
	}
	if cfg.Remote.Path != "~/workspace/work/nested/example" {
		t.Fatalf("remote path = %q", cfg.Remote.Path)
	}
	if cfg.Mapping.RelativePath != "work/nested/example" {
		t.Fatalf("relative path = %q", cfg.Mapping.RelativePath)
	}
}

func TestResolveConfigUsesGlobalMappingAndNodeDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "src", "work", "example")
	mustMkdir(t, repo)
	writeGlobal(t, home, `nodes:
  core-dev:
    ssh: core-dev.internal
    workspace_root: ~/workspace
defaults:
  ignores:
    - tmp
mapping:
  local_root: ~/src
`)

	cfg, err := ResolveConfig(Workspace{Name: "example", Root: repo})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Remote.Host != "core-dev.internal" {
		t.Fatalf("remote host = %q", cfg.Remote.Host)
	}
	if cfg.Remote.Path != "~/workspace/work/example" {
		t.Fatalf("remote path = %q", cfg.Remote.Path)
	}
	assertContains(t, cfg.Sync.Ignores, "tmp")
}

func TestResolveConfigWorkspaceOverridePrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "remote", "work", "example")
	mustMkdir(t, repo)
	writeGlobal(t, home, `default_node: core-dev
nodes:
  core-dev:
    ssh: core-dev
    workspace_root: ~/workspace
  gpu-dev:
    ssh: gpu.internal
    workspace_root: ~/gpu-workspace
defaults:
  ignores:
    - dist
mapping:
  local_root: ~/remote
`)
	writeFile(t, filepath.Join(repo, LocalOverrideFile), `workspace: steel-api
remote:
  node: gpu-dev
  path: ~/workspace/work/custom-location
sync:
  ignores:
    - .env.local
`)

	cfg, err := ResolveConfig(Workspace{Name: "example", Root: repo})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Workspace.Name != "steel-api" {
		t.Fatalf("workspace name = %q", cfg.Workspace.Name)
	}
	if cfg.Remote.Node != "gpu-dev" || cfg.Remote.Host != "gpu.internal" {
		t.Fatalf("remote node/host = %q/%q", cfg.Remote.Node, cfg.Remote.Host)
	}
	if cfg.Remote.Path != "~/workspace/work/custom-location" {
		t.Fatalf("remote path = %q", cfg.Remote.Path)
	}
	if cfg.Mapping.ConventionBased {
		t.Fatal("expected explicit remote path to disable convention-based path")
	}
	assertContains(t, cfg.Sync.Ignores, ".env.local")
}

func TestResolveConfigForcesGitIgnore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "remote", "work", "example")
	mustMkdir(t, repo)
	writeGlobal(t, home, `defaults:
  ignores:
    - .git
    - node_modules
mapping:
  local_root: ~/remote
`)
	writeFile(t, filepath.Join(repo, LocalOverrideFile), `sync:
  ignores:
    - .git
    - build
`)

	cfg, err := ResolveConfig(Workspace{Name: "example", Root: repo})
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Sync.Ignores) == 0 || cfg.Sync.Ignores[0] != ".git" {
		t.Fatalf(".git must be forced first, got %#v", cfg.Sync.Ignores)
	}
	if countValue(cfg.Sync.Ignores, ".git") != 1 {
		t.Fatalf(".git must be deduplicated, got %#v", cfg.Sync.Ignores)
	}
}

func TestLoadConfigMissingUsesTypedError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := LoadConfig("missing")
	if !apperrors.Is(err, apperrors.ErrWorkspaceConfigMissing) {
		t.Fatalf("expected ErrWorkspaceConfigMissing, got %v", err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeGlobal(t *testing.T, home string, content string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "devsync")
	mustMkdir(t, dir)
	writeFile(t, filepath.Join(dir, GlobalConfigFile), content)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, values []string, target string) {
	t.Helper()
	for _, value := range values {
		if value == target {
			return
		}
	}
	t.Fatalf("expected %#v to contain %q", values, target)
}

func countValue(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}
