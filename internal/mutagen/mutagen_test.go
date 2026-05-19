package mutagen

import (
	"reflect"
	"testing"

	"github.com/danew/devsync/internal/workspace"
)

func TestCreateArgsIncludesNameIgnoresAndEndpoints(t *testing.T) {
	cfg := workspace.Config{
		Workspace: workspace.WorkspaceIdentity{Name: "steel-api"},
		Remote:    workspace.RemoteConfig{Host: "core-dev", Path: "~/workspace/work/steel-api"},
		Sync:      workspace.SyncConfig{Ignores: []string{"node_modules", ".git", "dist"}},
	}

	got := CreateArgs("/local/steel-api", cfg)
	want := []string{
		"sync", "create", "--name", "devsync-steel-api",
		"--ignore", ".git",
		"--ignore", "node_modules",
		"--ignore", "dist",
		"/local/steel-api", "core-dev:~/workspace/work/steel-api",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CreateArgs() = %#v, want %#v", got, want)
	}
}

func TestParseListOutputDetectsPausedAndProblems(t *testing.T) {
	state := ParseListOutput("devsync-steel-api", `Name: devsync-steel-api
Status: Paused
Synchronization mode: Two Way Safe
Problems:
  beta scan error: permission denied`)

	if !state.Exists {
		t.Fatal("expected session to exist")
	}
	if !state.Paused || state.Active {
		t.Fatalf("expected paused inactive state, got paused=%v active=%v", state.Paused, state.Active)
	}
	if state.Healthy {
		t.Fatal("expected unhealthy state")
	}
	if len(state.Problems) == 0 {
		t.Fatal("expected problem lines")
	}
}

func TestParseListOutputNormalizesEndpointsAndIgnores(t *testing.T) {
	state := ParseListOutput("devsync-steel-api", `Name: devsync-steel-api
Status: Watching for changes
Alpha: /local/steel-api
Beta: core-dev:~/workspace/work/steel-api
Ignores: .git, node_modules, dist`)

	if state.Status != StatusRunning {
		t.Fatalf("status = %s", state.Status)
	}
	if state.Alpha != "/local/steel-api" || state.Beta != "core-dev:~/workspace/work/steel-api" {
		t.Fatalf("endpoints = %q/%q", state.Alpha, state.Beta)
	}
	if !state.Healthy {
		t.Fatal("expected healthy state")
	}
}

func TestReconcileDetectsEndpointAndIgnoreDrift(t *testing.T) {
	cfg := workspace.Config{
		Workspace: workspace.WorkspaceIdentity{Name: "steel-api"},
		Remote:    workspace.RemoteConfig{Host: "core-dev", Path: "~/workspace/work/steel-api"},
		Sync:      workspace.SyncConfig{Ignores: []string{".git", "node_modules", "dist"}},
	}
	state := State{Exists: true, Alpha: "/wrong", Beta: "core-dev:~/workspace/work/steel-api", Ignores: []string{".git"}}

	reconciliation := Reconcile(workspace.Workspace{Root: "/local/steel-api"}, cfg, state)
	if !reconciliation.Needed {
		t.Fatal("expected reconciliation to be needed")
	}
	if len(reconciliation.Reasons) < 2 {
		t.Fatalf("expected endpoint and ignore drift reasons, got %#v", reconciliation.Reasons)
	}
}
