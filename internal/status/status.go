package status

import (
	"github.com/danew/devsync/internal/apperrors"
	"github.com/danew/devsync/internal/git"
	"github.com/danew/devsync/internal/mutagen"
	"github.com/danew/devsync/internal/syncstate"
	"github.com/danew/devsync/internal/workspace"
)

type Report struct {
	Workspace workspace.Workspace
	Config    workspace.Config
	Local     git.State
	Remote    git.State
	Compare   git.Comparison
	Sync      mutagen.State
	Persisted syncstate.State
	Reconcile mutagen.Reconciliation
	Safe      bool
	Action    string
	Err       error
}

func Evaluate(ws workspace.Workspace, cfg workspace.Config, local git.State, remote git.State, comparison git.Comparison, sync mutagen.State, persisted syncstate.State, reconciliation mutagen.Reconciliation) Report {
	report := Report{Workspace: ws, Config: cfg, Local: local, Remote: remote, Compare: comparison, Sync: sync, Persisted: persisted, Reconcile: reconciliation}

	switch {
	case local.Branch == "":
		report.Action = "local repository is in detached HEAD state; checkout a branch before syncing"
		report.Err = apperrors.New(apperrors.ErrDetachedHead, report.Action)
	case remote.Branch == "":
		report.Action = "remote repository is in detached HEAD state; checkout a branch before syncing"
		report.Err = apperrors.New(apperrors.ErrDetachedHead, report.Action)
	case local.Branch != remote.Branch:
		report.Action = "branch mismatch; checkout matching branches manually before syncing"
		report.Err = apperrors.New(apperrors.ErrBranchMismatch, report.Action)
	case !comparison.Known:
		if comparison.Err != nil {
			report.Action = comparison.Err.Error()
			report.Err = comparison.Err
		} else {
			report.Action = "history comparison is unknown; aborting safely"
			report.Err = apperrors.New(apperrors.ErrHistoryUnknown, report.Action)
		}
	case comparison.LocalAhead > 0 && comparison.RemoteAhead > 0:
		report.Action = "workspace histories diverged; resolve manually before syncing"
		report.Err = apperrors.New(apperrors.ErrHistoryDiverged, report.Action)
	case reconciliation.Needed:
		report.Action = "mutagen session configuration drift detected; inspect and recreate manually if intended"
		report.Err = apperrors.NewWithRemedy(apperrors.ErrSessionDrift, report.Action, reconciliation.Remedy)
	case sync.Exists && !sync.Healthy:
		report.Action = "mutagen session is unhealthy; inspect synchronization problems before syncing"
		report.Err = apperrors.NewWithRemedy(apperrors.ErrMutagenUnhealthy, report.Action, "run devsync doctor and mutagen sync list "+sync.SessionName)
	case comparison.LocalAhead > 0:
		report.Safe = true
		report.Action = "safe to push local commits before synchronization"
	case comparison.RemoteAhead > 0:
		report.Safe = true
		report.Action = "safe to pull remote commits before synchronization"
	default:
		report.Safe = true
		report.Action = "safe to synchronize working tree changes"
	}

	return report
}
