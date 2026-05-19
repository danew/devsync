package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/danew/devsync/internal/apperrors"
	"github.com/danew/devsync/internal/git"
	"github.com/danew/devsync/internal/mutagen"
	devssh "github.com/danew/devsync/internal/ssh"
	"github.com/danew/devsync/internal/status"
	"github.com/danew/devsync/internal/ui"
	"github.com/danew/devsync/internal/workspace"
)

func Execute() error {
	root := newRootCommand()
	return root.Execute()
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "devsync",
		Short:         "Git-aware workspace synchronization for remote-first development",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd.Context(), os.Stdout)
		},
	}

	root.AddCommand(newStatusCommand())
	root.AddCommand(newSyncCommand())
	root.AddCommand(newInitCommand())
	root.AddCommand(&cobra.Command{
		Use:   "attach",
		Short: "Attach to a Mutagen sync session (not implemented in v1 scaffold)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("attach is intentionally not implemented yet")
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "detach",
		Short: "Detach from a Mutagen sync session (not implemented in v1 scaffold)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("detach is intentionally not implemented yet")
		},
	})

	return root
}

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Inspect local, remote, and synchronization state",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := buildStatus(cmd.Context())
			if err != nil {
				return err
			}
			ui.WriteStatus(os.Stdout, report)
			return nil
		},
	}
}

func newSyncCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Validate Git state and prepare safe workspace synchronization",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd.Context(), os.Stdout)
		},
	}
}

func newInitCommand() *cobra.Command {
	var remoteHost string
	var remotePath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create workspace configuration outside the repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := workspace.Discover(cmd.Context())
			if err != nil {
				return err
			}
			if remoteHost == "" || remotePath == "" {
				return fmt.Errorf("init requires --remote-host and --remote-path")
			}
			cfg := workspace.DefaultConfig(ws.Name)
			cfg.Remote.Node = remoteHost
			cfg.Remote.Host = remoteHost
			cfg.Remote.Path = remotePath
			path, err := workspace.WriteLocalOverride(ws.Root, cfg)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "Created config: %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&remoteHost, "remote-host", "", "SSH host for the remote workspace")
	cmd.Flags().StringVar(&remotePath, "remote-path", "", "Path to the remote Git repository")
	return cmd
}

func runSync(ctx context.Context, out *os.File) error {
	report, err := buildStatus(ctx)
	if err != nil {
		return err
	}
	ui.WriteStatus(out, report)
	if !report.Safe {
		return report.Err
	}

	gitMutated := false
	switch {
	case report.Compare.LocalAhead > 0:
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Git: pushing %s to %s:%s\n", report.Local.Branch, report.Config.Remote.Host, report.Config.Remote.Path)
		if err := git.PushBranch(ctx, report.Workspace.Root, report.Config.Remote.Host, report.Config.Remote.Path, report.Local.Branch); err != nil {
			return err
		}
		gitMutated = true
	case report.Compare.RemoteAhead > 0:
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Git: pulling %s from %s:%s with --ff-only\n", report.Local.Branch, report.Config.Remote.Host, report.Config.Remote.Path)
		if err := git.PullBranchFastForward(ctx, report.Workspace.Root, report.Config.Remote.Host, report.Config.Remote.Path, report.Local.Branch); err != nil {
			return err
		}
		gitMutated = true
	}
	if gitMutated {
		report, err = buildStatus(ctx)
		if err != nil {
			return err
		}
		if !report.Safe {
			return report.Err
		}
	}

	runner := mutagen.CLIRunner{}
	syncState, err := mutagen.EnsureSession(ctx, runner, report.Workspace, report.Config)
	if err != nil {
		return err
	}
	if syncState.Paused {
		fmt.Fprintf(out, "Mutagen: resuming session %s\n", syncState.SessionName)
		if err := mutagen.Resume(ctx, runner, syncState.SessionName); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Mutagen: flushing session %s\n", syncState.SessionName)
	if err := mutagen.Flush(ctx, runner, syncState.SessionName); err != nil {
		return err
	}
	syncState, err = mutagen.InspectWithRunner(ctx, runner, report.Config.Workspace.Name)
	if err != nil {
		return err
	}
	if !syncState.Healthy {
		return apperrors.New(apperrors.ErrMutagenUnhealthy, "mutagen session is not healthy after flush; inspect with mutagen sync list "+syncState.SessionName)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Sync complete.")
	return nil
}

func buildStatus(ctx context.Context) (status.Report, error) {
	ws, err := workspace.Discover(ctx)
	if err != nil {
		return status.Report{}, err
	}
	cfg, err := workspace.ResolveConfig(ws)
	if err != nil {
		return status.Report{}, err
	}

	local, err := git.InspectLocal(ctx, ws.Root)
	if err != nil {
		return status.Report{}, err
	}
	remoteRunner := devssh.Runner{Host: cfg.Remote.Host}
	remote, err := git.InspectRemote(ctx, remoteRunner, cfg.Remote.Path)
	if err != nil {
		return status.Report{}, err
	}
	comparison := git.CompareHistories(ctx, ws.Root, local.Head, cfg.Remote.Host, cfg.Remote.Path, remote.Head)
	syncState := mutagen.Inspect(ctx, cfg.Workspace.Name)

	return status.Evaluate(ws, cfg, local, remote, comparison, syncState), nil
}
