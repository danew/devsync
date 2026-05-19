package plan

import (
	"fmt"

	"github.com/danew/devsync/internal/status"
)

type Kind string

const (
	Check       Kind = "check"
	GitMutation Kind = "git"
	MutagenOp   Kind = "mutagen"
	Lock        Kind = "lock"
	Reconcile   Kind = "reconcile"
)

type Operation struct {
	Kind        Kind
	Description string
	Mutates     bool
}

type Plan struct {
	Workspace string
	DryRun    bool
	Ops       []Operation
}

func FromReport(report status.Report, dryRun bool) Plan {
	ops := []Operation{{Kind: Lock, Description: "acquire local workspace lock", Mutates: false}}
	ops = append(ops, Operation{Kind: Check, Description: "validate branch equality and ancestry safety", Mutates: false})
	if report.Reconcile.Needed {
		ops = append(ops, Operation{Kind: Reconcile, Description: "manual Mutagen session reconciliation required", Mutates: false})
		return Plan{Workspace: report.Config.Workspace.Name, DryRun: dryRun, Ops: ops}
	}
	switch {
	case report.Compare.LocalAhead > 0:
		ops = append(ops, Operation{Kind: GitMutation, Description: fmt.Sprintf("push %s to %s:%s", report.Local.Branch, report.Config.Remote.Host, report.Config.Remote.Path), Mutates: true})
	case report.Compare.RemoteAhead > 0:
		ops = append(ops, Operation{Kind: GitMutation, Description: fmt.Sprintf("pull %s from %s:%s with --ff-only", report.Local.Branch, report.Config.Remote.Host, report.Config.Remote.Path), Mutates: true})
	default:
		ops = append(ops, Operation{Kind: Check, Description: "no Git ref update required", Mutates: false})
	}
	if report.Sync.Exists {
		ops = append(ops, Operation{Kind: MutagenOp, Description: "reuse Mutagen session " + report.Sync.SessionName, Mutates: false})
	} else {
		ops = append(ops, Operation{Kind: MutagenOp, Description: "create Mutagen session " + report.Sync.SessionName, Mutates: true})
	}
	if report.Sync.Paused {
		ops = append(ops, Operation{Kind: MutagenOp, Description: "resume Mutagen session", Mutates: true})
	}
	ops = append(ops, Operation{Kind: MutagenOp, Description: "flush Mutagen session", Mutates: true})
	ops = append(ops, Operation{Kind: Check, Description: "verify post-flush health", Mutates: false})
	return Plan{Workspace: report.Config.Workspace.Name, DryRun: dryRun, Ops: ops}
}
