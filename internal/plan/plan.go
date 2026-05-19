package plan

import (
	"fmt"

	"github.com/danew/devsync/internal/status"
)

type Kind string

const (
	Check     Kind = "check"
	MutagenOp Kind = "mutagen"
	Lock      Kind = "lock"
	Reconcile Kind = "reconcile"
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
		ops = append(ops, Operation{Kind: Check, Description: fmt.Sprintf("manual Git history synchronization required: local %s is ahead of remote workspace", report.Local.Branch), Mutates: false})
	case report.Compare.RemoteAhead > 0:
		ops = append(ops, Operation{Kind: Check, Description: fmt.Sprintf("manual Git history synchronization required: remote workspace %s is ahead of local", report.Local.Branch), Mutates: false})
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
