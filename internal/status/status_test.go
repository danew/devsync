package status

import (
	"testing"

	"github.com/danew/devsync/internal/apperrors"
	"github.com/danew/devsync/internal/git"
	"github.com/danew/devsync/internal/mutagen"
	"github.com/danew/devsync/internal/syncstate"
	"github.com/danew/devsync/internal/workspace"
)

func TestEvaluateRejectsBranchMismatch(t *testing.T) {
	report := Evaluate(workspace.Workspace{}, workspace.Config{}, git.State{Branch: "feature"}, git.State{Branch: "main"}, git.Comparison{Known: true}, mutagen.State{}, syncstate.State{}, mutagen.Reconciliation{})

	if report.Safe {
		t.Fatal("branch mismatch must not be safe")
	}
	if !apperrors.Is(report.Err, apperrors.ErrBranchMismatch) {
		t.Fatalf("expected ErrBranchMismatch, got %v", report.Err)
	}
}

func TestEvaluateRejectsDivergence(t *testing.T) {
	report := Evaluate(workspace.Workspace{}, workspace.Config{}, git.State{Branch: "main"}, git.State{Branch: "main"}, git.Comparison{Known: true, LocalAhead: 2, RemoteAhead: 4}, mutagen.State{}, syncstate.State{}, mutagen.Reconciliation{})

	if report.Safe {
		t.Fatal("divergence must not be safe")
	}
	if !apperrors.Is(report.Err, apperrors.ErrHistoryDiverged) {
		t.Fatalf("expected ErrHistoryDiverged, got %v", report.Err)
	}
}

func TestEvaluateAllowsRemoteAhead(t *testing.T) {
	report := Evaluate(workspace.Workspace{}, workspace.Config{}, git.State{Branch: "main"}, git.State{Branch: "main"}, git.Comparison{Known: true, RemoteAhead: 2}, mutagen.State{}, syncstate.State{}, mutagen.Reconciliation{})

	if !report.Safe {
		t.Fatalf("remote-ahead fast-forward should be safe: %v", report.Err)
	}
}

func TestEvaluateRejectsSessionDrift(t *testing.T) {
	report := Evaluate(workspace.Workspace{}, workspace.Config{}, git.State{Branch: "main"}, git.State{Branch: "main"}, git.Comparison{Known: true}, mutagen.State{}, syncstate.State{}, mutagen.Reconciliation{Needed: true, Reasons: []string{"endpoint drift"}})

	if report.Safe {
		t.Fatal("session drift must not be safe")
	}
	if !apperrors.Is(report.Err, apperrors.ErrSessionDrift) {
		t.Fatalf("expected ErrSessionDrift, got %v", report.Err)
	}
}

func TestEvaluateMarksInitialSyncRiskForDirtyTrees(t *testing.T) {
	report := Evaluate(workspace.Workspace{}, workspace.Config{}, git.State{Branch: "main", Dirty: true}, git.State{Branch: "main"}, git.Comparison{Known: true}, mutagen.State{Exists: false}, syncstate.State{}, mutagen.Reconciliation{})

	if !report.Safe {
		t.Fatalf("initial sync risk should be visible but status-safe: %v", report.Err)
	}
	if !report.Initial.Pending || !report.Initial.Risky {
		t.Fatalf("expected risky initial sync, got %#v", report.Initial)
	}
}

func TestEvaluateMarksCleanInitialSyncPending(t *testing.T) {
	report := Evaluate(workspace.Workspace{}, workspace.Config{}, git.State{Branch: "main"}, git.State{Branch: "main"}, git.Comparison{Known: true}, mutagen.State{Exists: false}, syncstate.State{}, mutagen.Reconciliation{})

	if !report.Initial.Pending || report.Initial.Risky {
		t.Fatalf("expected clean initial sync pending, got %#v", report.Initial)
	}
}

func TestEvaluateRejectsMutagenConflictsWithGuidance(t *testing.T) {
	report := Evaluate(workspace.Workspace{}, workspace.Config{}, git.State{Branch: "main"}, git.State{Branch: "main"}, git.Comparison{Known: true}, mutagen.State{Exists: true, Conflicts: []string{"file.go"}}, syncstate.State{}, mutagen.Reconciliation{})

	if report.Safe {
		t.Fatal("mutagen conflicts must not be safe")
	}
	if !apperrors.Is(report.Err, apperrors.ErrMutagenUnhealthy) {
		t.Fatalf("expected ErrMutagenUnhealthy, got %v", report.Err)
	}
}
