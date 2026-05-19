package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

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
			cfg.Remote.Host = remoteHost
			cfg.Remote.Path = remotePath
			path, err := workspace.WriteConfig(cfg)
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
		return errors.New(report.Action)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Sync orchestration is not implemented yet; Git state passed v1 safety checks.")
	return nil
}

func buildStatus(ctx context.Context) (status.Report, error) {
	ws, err := workspace.Discover(ctx)
	if err != nil {
		return status.Report{}, err
	}
	cfg, err := workspace.LoadConfig(ws.Name)
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
	syncState := mutagen.Inspect(ctx, cfg.Workspace)

	return status.Evaluate(ws, cfg, local, remote, comparison, syncState), nil
}
