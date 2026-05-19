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
	Initial   InitialSync
	Safe      bool
	Action    string
	Err       error
}

type InitialSync struct {
	Pending bool
	Risky   bool
	Reasons []string
}

func Evaluate(ws workspace.Workspace, cfg workspace.Config, local git.State, remote git.State, comparison git.Comparison, sync mutagen.State, persisted syncstate.State, reconciliation mutagen.Reconciliation) Report {
	report := Report{Workspace: ws, Config: cfg, Local: local, Remote: remote, Compare: comparison, Sync: sync, Persisted: persisted, Reconcile: reconciliation}
	report.Initial = evaluateInitialSync(local, remote, comparison, sync)

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
	case comparison.LocalAhead > 0:
		report.Action = "local workspace contains commits not present remotely; synchronize Git history manually using canonical Git remotes before running sync"
		report.Err = apperrors.NewWithRemedy(apperrors.ErrHistoryOutOfSync, report.Action, "push or otherwise publish commits with your repository's canonical remote, then update the remote workspace manually")
	case comparison.RemoteAhead > 0:
		report.Action = "remote workspace contains commits not present locally; synchronize Git history manually using canonical Git remotes before running sync"
		report.Err = apperrors.NewWithRemedy(apperrors.ErrHistoryOutOfSync, report.Action, "for a typical origin/main setup: git pull --ff-only origin main")
	case reconciliation.Needed:
		report.Action = "mutagen session configuration drift detected; inspect and recreate manually if intended"
		report.Err = apperrors.NewWithRemedy(apperrors.ErrSessionDrift, report.Action, reconciliation.Remedy)
	case sync.Exists && len(sync.Conflicts) > 0:
		report.Action = "mutagen reported filesystem conflicts; reconcile working trees manually before syncing"
		report.Err = apperrors.NewWithRemedy(apperrors.ErrMutagenUnhealthy, report.Action, "terminate the session, reconcile affected files manually, then rerun devsync sync --dry-run")
	case sync.Exists && !sync.Healthy:
		report.Action = "mutagen session is unhealthy; inspect synchronization problems before syncing"
		report.Err = apperrors.NewWithRemedy(apperrors.ErrMutagenUnhealthy, report.Action, "run devsync doctor and mutagen sync list "+sync.SessionName)
	default:
		report.Safe = true
		report.Action = "safe to synchronize working tree changes"
	}

	return report
}

func evaluateInitialSync(local git.State, remote git.State, comparison git.Comparison, sync mutagen.State) InitialSync {
	if sync.Exists {
		return InitialSync{}
	}
	initial := InitialSync{Pending: true}
	if local.Dirty {
		initial.Risky = true
		initial.Reasons = append(initial.Reasons, "local working tree is dirty")
	}
	if remote.Dirty {
		initial.Risky = true
		initial.Reasons = append(initial.Reasons, "remote working tree is dirty")
	}
	if comparison.Known && (comparison.LocalAhead > 0 || comparison.RemoteAhead > 0) {
		initial.Risky = true
		initial.Reasons = append(initial.Reasons, "local and remote HEADs are not equal yet")
	}
	return initial
}
