package ui

import (
	"fmt"
	"io"

	"github.com/danew/devsync/internal/status"
)

func WriteStatus(w io.Writer, report status.Report) {
	fmt.Fprintf(w, "Workspace: %s\n", report.Config.Workspace)
	fmt.Fprintf(w, "Local root: %s\n", report.Workspace.Root)
	fmt.Fprintf(w, "Remote: %s:%s\n", report.Config.Remote.Host, report.Config.Remote.Path)
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Branch: %s\n", valueOrUnknown(report.Local.Branch))
	if report.Local.Branch != report.Remote.Branch {
		fmt.Fprintf(w, "Remote branch: %s\n", valueOrUnknown(report.Remote.Branch))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Local:")
	fmt.Fprintf(w, "  %s\n", cleanLabel(report.Local.Dirty))
	fmt.Fprintf(w, "  %s\n", aheadLabel(report.Compare.LocalAhead, "ahead"))
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Remote:")
	fmt.Fprintf(w, "  %s\n", cleanLabel(report.Remote.Dirty))
	fmt.Fprintf(w, "  %s\n", aheadLabel(report.Compare.RemoteAhead, "ahead"))
	fmt.Fprintln(w)

	fmt.Fprintln(w, "History:")
	if report.Compare.Known {
		fmt.Fprintf(w, "  local ahead: %d\n", report.Compare.LocalAhead)
		fmt.Fprintf(w, "  remote ahead: %d\n", report.Compare.RemoteAhead)
	} else {
		fmt.Fprintln(w, "  unknown")
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Sync:")
	fmt.Fprintf(w, "  session: %s\n", report.Sync.SessionName)
	if report.Sync.Active {
		fmt.Fprintln(w, "  active")
	} else {
		fmt.Fprintln(w, "  inactive")
	}
	if report.Sync.Healthy {
		fmt.Fprintln(w, "  healthy")
	} else if report.Sync.Message != "" {
		fmt.Fprintf(w, "  %s\n", report.Sync.Message)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Action:")
	fmt.Fprintf(w, "  %s\n", report.Action)
}

func cleanLabel(dirty bool) string {
	if dirty {
		return "dirty working tree"
	}
	return "clean"
}

func aheadLabel(count int, label string) string {
	if count == 0 {
		return "up-to-date"
	}
	return fmt.Sprintf("%s by %d", label, count)
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
