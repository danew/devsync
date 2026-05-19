package mutagen

import (
	"reflect"
	"testing"

	devssh "github.com/danew/devsync/internal/ssh"
	"github.com/danew/devsync/internal/workspace"
)

func TestCreateArgsIncludesNameIgnoresAndEndpoints(t *testing.T) {
	cfg := workspace.Config{
		Workspace: workspace.WorkspaceIdentity{Name: "steel-api"},
		Remote:    workspace.RemoteConfig{Host: "core-dev", Path: "~/workspace/work/steel-api", Target: devssh.ParseTarget("core-dev")},
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

func TestCreateArgsUsesCanonicalNormalizedTarget(t *testing.T) {
	cfg := workspace.Config{
		Workspace: workspace.WorkspaceIdentity{Name: "fly-metadata"},
		Remote: workspace.RemoteConfig{
			Host:   "core-dev",
			Path:   "/home/dev/workspace/work/fly-metadata",
			Target: devssh.Target{User: "dev", Host: "100.72.16.64"},
		},
		Sync: workspace.SyncConfig{Ignores: []string{".git"}},
	}

	got := CreateArgs("/local/fly-metadata", cfg)
	wantEndpoint := "dev@100.72.16.64:/home/dev/workspace/work/fly-metadata"
	if got[len(got)-1] != wantEndpoint {
		t.Fatalf("endpoint = %q, want %q (args %#v)", got[len(got)-1], wantEndpoint, got)
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

func TestParseListOutputExtractsLongEndpointURLsAndConflicts(t *testing.T) {
	state := ParseListOutput("devsync-steel-api", `Name: devsync-steel-api
Alpha:
	URL: /local/steel-api
Beta:
	URL: core-dev:~/workspace/work/steel-api
Conflicts:
	(alpha) internal/workspace/config.go (<non-existent> -> File (abc))
	(beta)  internal/workspace/config.go (<non-existent> -> File (def))
Status: Watching for changes`)

	if state.Alpha != "/local/steel-api" || state.Beta != "core-dev:~/workspace/work/steel-api" {
		t.Fatalf("endpoints = %q/%q", state.Alpha, state.Beta)
	}
	if len(state.Conflicts) != 1 || state.Conflicts[0] != "internal/workspace/config.go" {
		t.Fatalf("conflicts = %#v", state.Conflicts)
	}
	if state.Healthy {
		t.Fatal("conflicts should mark session unhealthy")
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
