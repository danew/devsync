package plan

import (
	"testing"

	"github.com/danew/devsync/internal/git"
	"github.com/danew/devsync/internal/mutagen"
	"github.com/danew/devsync/internal/status"
	"github.com/danew/devsync/internal/workspace"
)

func TestFromReportDoesNotPlanGitHistoryMutation(t *testing.T) {
	report := status.Report{
		Config: workspace.Config{
			Workspace: workspace.WorkspaceIdentity{Name: "steel-api"},
			Remote:    workspace.RemoteConfig{Host: "core-dev", Path: "~/workspace/work/steel-api"},
		},
		Local:   git.State{Branch: "main"},
		Compare: git.Comparison{Known: true, LocalAhead: 1},
		Sync:    mutagen.State{SessionName: "devsync-steel-api", Exists: false},
	}

	plan := FromReport(report, true)
	if !plan.DryRun {
		t.Fatal("expected dry-run plan")
	}
	if len(plan.Ops) == 0 {
		t.Fatal("expected operations")
	}
	if contains(plan, Kind("git")) {
		t.Fatalf("peer-clone history changes must not be planned as Git mutations: %#v", plan.Ops)
	}
	if !contains(plan, MutagenOp) || !contains(plan, Lock) {
		t.Fatalf("expected mutagen and lock operations, got %#v", plan.Ops)
	}
}

func contains(plan Plan, kind Kind) bool {
	for _, op := range plan.Ops {
		if op.Kind == kind {
			return true
		}
	}
	return false
}
