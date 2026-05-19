package status

import (
	"github.com/danew/devsync/internal/git"
	"github.com/danew/devsync/internal/mutagen"
	"github.com/danew/devsync/internal/workspace"
)

type Report struct {
	Workspace workspace.Workspace
	Config    workspace.Config
	Local     git.State
	Remote    git.State
	Compare   git.Comparison
	Sync      mutagen.State
	Safe      bool
	Action    string
}

func Evaluate(ws workspace.Workspace, cfg workspace.Config, local git.State, remote git.State, comparison git.Comparison, sync mutagen.State) Report {
	report := Report{Workspace: ws, Config: cfg, Local: local, Remote: remote, Compare: comparison, Sync: sync}

	switch {
	case local.Branch == "":
		report.Action = "local repository is in detached HEAD state; checkout a branch before syncing"
	case remote.Branch == "":
		report.Action = "remote repository is in detached HEAD state; checkout a branch before syncing"
	case local.Branch != remote.Branch:
		report.Action = "branch mismatch; checkout matching branches manually before syncing"
	case !comparison.Known:
		if comparison.Err != nil {
			report.Action = comparison.Err.Error()
		} else {
			report.Action = "history comparison is unknown; aborting safely"
		}
	case comparison.LocalAhead > 0 && comparison.RemoteAhead > 0:
		report.Action = "workspace histories diverged; resolve manually before syncing"
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
