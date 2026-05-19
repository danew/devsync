package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/danew/devsync/internal/status"
)

func WriteStatus(w io.Writer, report status.Report) {
	fmt.Fprintf(w, "Workspace: %s\n", report.Config.Workspace.Name)
	fmt.Fprintf(w, "Local root: %s\n", report.Workspace.Root)
	fmt.Fprintf(w, "Remote: %s:%s\n", report.Config.Remote.Host, report.Config.Remote.Path)
	fmt.Fprintf(w, "Remote node: %s\n", report.Config.Remote.Node)
	fmt.Fprintf(w, "Mapping: %s -> %s\n", report.Config.Mapping.LocalRoot, report.Config.Mapping.RemoteRoot)
	fmt.Fprintf(w, "Config: global=%s workspace_override=%s\n", loadedLabel(report.Config.Sources.GlobalLoaded), loadedLabel(report.Config.Sources.LocalOverrideFound))
	fmt.Fprintf(w, "Resolved from: node=%s path=%s ignores=%s\n", report.Config.Sources.RemoteNodeSource, report.Config.Sources.RemotePathSource, report.Config.Sources.IgnoreSource)
	fmt.Fprintf(w, "Ignores: %s\n", joinValues(report.Config.Sync.Ignores))
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
	if report.Sync.Exists {
		fmt.Fprintln(w, "  exists")
	} else {
		fmt.Fprintln(w, "  missing")
	}
	if report.Sync.Active {
		fmt.Fprintln(w, "  running")
	} else if report.Sync.Paused {
		fmt.Fprintln(w, "  paused")
	} else {
		fmt.Fprintln(w, "  inactive")
	}
	if report.Sync.Healthy {
		fmt.Fprintln(w, "  healthy")
	} else {
		fmt.Fprintln(w, "  health unknown or unhealthy")
	}
	if report.Sync.LastDirection != "" {
		fmt.Fprintf(w, "  direction: %s\n", report.Sync.LastDirection)
	}
	if len(report.Sync.Problems) > 0 {
		fmt.Fprintln(w, "  problems:")
		for _, problem := range report.Sync.Problems {
			fmt.Fprintf(w, "  - %s\n", problem)
		}
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

func loadedLabel(loaded bool) string {
	if loaded {
		return "loaded"
	}
	return "not found"
}

func joinValues(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
