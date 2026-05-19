package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/danew/devsync/internal/apperrors"
	"github.com/danew/devsync/internal/buildinfo"
	"github.com/danew/devsync/internal/git"
	"github.com/danew/devsync/internal/lock"
	"github.com/danew/devsync/internal/logging"
	"github.com/danew/devsync/internal/mutagen"
	"github.com/danew/devsync/internal/plan"
	devssh "github.com/danew/devsync/internal/ssh"
	"github.com/danew/devsync/internal/status"
	"github.com/danew/devsync/internal/syncstate"
	"github.com/danew/devsync/internal/ui"
	"github.com/danew/devsync/internal/workspace"
)

type cliOptions struct {
	debug bool
	trace bool
}

func Execute() error {
	root := newRootCommand()
	return root.Execute()
}

func newRootCommand() *cobra.Command {
	options := &cliOptions{}
	root := &cobra.Command{
		Use:           "devsync",
		Short:         "Git-aware workspace synchronization for remote-first development",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logging.New(options.debug, options.trace, os.Stderr)
			return runSync(cmd.Context(), os.Stdout, false, logger)
		},
	}
	root.PersistentFlags().BoolVar(&options.debug, "debug", false, "enable structured debug logging")
	root.PersistentFlags().BoolVar(&options.trace, "trace", false, "enable verbose trace logging")

	root.AddCommand(newStatusCommand())
	root.AddCommand(newSyncCommand(options))
	root.AddCommand(newDoctorCommand())
	root.AddCommand(newBootstrapCommand())
	root.AddCommand(newInitRemoteCommand())
	root.AddCommand(newVersionCommand())
	root.AddCommand(newSessionCommand())
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

func newSyncCommand(options *cliOptions) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Validate Git state and prepare safe workspace synchronization",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logging.New(options.debug, options.trace, os.Stderr)
			return runSync(cmd.Context(), os.Stdout, dryRun, logger)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show planned operations without mutating Git or Mutagen state")
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build and runtime version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := buildinfo.Current()
			fmt.Fprintf(os.Stdout, "devsync %s\ncommit: %s\nbuilt: %s\ngo: %s\nplatform: %s/%s\n", info.Version, info.Commit, info.Date, info.GoVersion, info.OS, info.Arch)
			return nil
		},
	}
}

func newBootstrapCommand() *cobra.Command {
	var initWorkspace bool
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Validate first-run setup and generate missing local configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBootstrap(cmd.Context(), os.Stdout, initWorkspace)
		},
	}
	cmd.Flags().BoolVar(&initWorkspace, "init-workspace", false, "write .devsync.yaml for the current repository using resolved conventions")
	return cmd
}

func newInitRemoteCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "init-remote",
		Short: "Explicitly seed the remote workspace repository and stop before synchronization",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInitRemote(cmd.Context(), os.Stdout, yes)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm remote initialization without prompting")
	return cmd
}

func newSessionCommand() *cobra.Command {
	session := &cobra.Command{Use: "session", Short: "Inspect and manage the Mutagen session for this workspace"}
	session.AddCommand(&cobra.Command{Use: "ls", Short: "List Mutagen sync sessions", RunE: func(cmd *cobra.Command, args []string) error {
		out, err := mutagen.List(cmd.Context(), mutagen.CLIRunner{})
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, out)
		return nil
	}})
	session.AddCommand(&cobra.Command{Use: "inspect", Short: "Inspect the resolved workspace session", RunE: func(cmd *cobra.Command, args []string) error {
		report, err := buildStatus(cmd.Context())
		if err != nil {
			return err
		}
		ui.WriteStatus(os.Stdout, report)
		return nil
	}})
	session.AddCommand(&cobra.Command{Use: "flush", Short: "Flush the resolved workspace session", RunE: func(cmd *cobra.Command, args []string) error {
		report, err := buildStatus(cmd.Context())
		if err != nil {
			return err
		}
		if !report.Sync.Exists {
			return apperrors.NewWithRemedy(apperrors.ErrMutagenUnhealthy, "Mutagen session does not exist", "run devsync sync --dry-run, then devsync sync to create it after Git safety checks pass")
		}
		return mutagen.Flush(cmd.Context(), mutagen.CLIRunner{}, report.Sync.SessionName)
	}})
	var force bool
	recreate := &cobra.Command{Use: "recreate", Short: "Explicitly recreate the resolved workspace session", RunE: func(cmd *cobra.Command, args []string) error {
		if !force {
			return fmt.Errorf("recreate requires --force-recreate-session")
		}
		report, err := buildStatus(cmd.Context())
		if err != nil {
			return err
		}
		runner := mutagen.CLIRunner{}
		if report.Sync.Exists {
			if err := mutagen.Terminate(cmd.Context(), runner, report.Sync.SessionName); err != nil {
				return err
			}
		}
		_, err = mutagen.EnsureSession(cmd.Context(), runner, report.Workspace, report.Config)
		return err
	}}
	recreate.Flags().BoolVar(&force, "force-recreate-session", false, "explicitly terminate and recreate the Mutagen session")
	session.AddCommand(recreate)
	return session
}

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Validate DevSync workspace prerequisites and operational health",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.Context(), os.Stdout)
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

func runSync(ctx context.Context, out io.Writer, dryRun bool, logger logging.Logger) error {
	logger.Debug("sync.start", map[string]string{"dry_run": fmt.Sprintf("%v", dryRun)})
	if err := ensureWorkspaceConfigured(ctx, out); err != nil {
		return err
	}
	report, err := buildStatus(ctx)
	if err != nil {
		return err
	}
	ui.WriteStatus(out, report)
	if !report.Safe {
		return report.Err
	}
	if dryRun {
		if report.Initial.Pending && report.Initial.Risky {
			writeInitialSyncWarning(out, report)
		}
		writePlan(out, plan.FromReport(report, true))
		return nil
	}
	workspaceLock, err := lock.Acquire(report.Config.Workspace.Name, "sync")
	if err != nil {
		return err
	}
	defer workspaceLock.Release()
	logger.Debug("sync.lock_acquired", map[string]string{"workspace": report.Config.Workspace.Name})

	gitMutated := false
	switch {
	case report.Compare.LocalAhead > 0:
		started := time.Now()
		if err := git.CheckRemoteBranchVisible(ctx, devssh.Runner{Target: report.Config.Remote.Target}, report.Config.Remote.Path, report.Local.Branch); err != nil {
			return err
		}
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Git: pushing %s to %s:%s\n", report.Local.Branch, report.Config.Remote.Host, report.Config.Remote.Path)
		if err := git.PushBranch(ctx, report.Workspace.Root, report.Config.Remote.Host, report.Config.Remote.Path, report.Local.Branch); err != nil {
			return err
		}
		logger.Debug("sync.git_push", map[string]string{"duration": time.Since(started).String()})
		gitMutated = true
	case report.Compare.RemoteAhead > 0:
		started := time.Now()
		if err := git.CheckRemoteBranchVisible(ctx, devssh.Runner{Target: report.Config.Remote.Target}, report.Config.Remote.Path, report.Local.Branch); err != nil {
			return err
		}
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Git: pulling %s from %s:%s with --ff-only\n", report.Local.Branch, report.Config.Remote.Host, report.Config.Remote.Path)
		if err := git.PullBranchFastForward(ctx, report.Workspace.Root, report.Config.Remote.Host, report.Config.Remote.Path, report.Local.Branch); err != nil {
			return err
		}
		logger.Debug("sync.git_pull", map[string]string{"duration": time.Since(started).String()})
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
	if err := confirmInitialSync(out, report); err != nil {
		return err
	}

	runner := mutagen.CLIRunner{}
	started := time.Now()
	syncState, err := mutagen.EnsureSession(ctx, runner, report.Workspace, report.Config)
	if err != nil {
		return err
	}
	logger.Debug("sync.ensure_session", map[string]string{"duration": time.Since(started).String(), "session": syncState.SessionName})
	if syncState.Paused {
		fmt.Fprintf(out, "Mutagen: resuming session %s\n", syncState.SessionName)
		if err := mutagen.Resume(ctx, runner, syncState.SessionName); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Mutagen: flushing session %s\n", syncState.SessionName)
	started = time.Now()
	if err := mutagen.Flush(ctx, runner, syncState.SessionName); err != nil {
		return err
	}
	logger.Debug("sync.flush", map[string]string{"duration": time.Since(started).String(), "session": syncState.SessionName})
	syncState, err = mutagen.InspectWithRunner(ctx, runner, report.Config.Workspace.Name)
	if err != nil {
		return err
	}
	if !syncState.Healthy {
		return apperrors.NewWithRemedy(apperrors.ErrMutagenUnhealthy, "mutagen session is not healthy after flush; inspect with mutagen sync list "+syncState.SessionName, "if conflicts are present: terminate the session, reconcile working trees manually, then recreate the session explicitly")
	}
	if err := syncstate.Save(syncstate.State{
		Workspace:       report.Config.Workspace.Name,
		SessionName:     syncState.SessionName,
		LastFlushAt:     time.Now().UTC(),
		LastDirection:   syncState.LastDirection,
		LastRemoteHost:  report.Config.Remote.Host,
		LastRemotePath:  report.Config.Remote.Path,
		LastLocalRoot:   report.Workspace.Root,
		LastMutagenMode: report.Config.Sync.Mode,
	}); err != nil {
		return err
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Sync complete.")
	return nil
}

func confirmInitialSync(out io.Writer, report status.Report) error {
	if !report.Initial.Pending || !report.Initial.Risky {
		return nil
	}
	if !isInteractive() {
		return initialSyncRiskError(report)
	}
	writeInitialSyncWarning(out, report)
	fmt.Fprint(out, "Proceed anyway? [y/N] ")
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		return apperrors.NewWithRemedy(apperrors.ErrInitialSyncRisk, "initial synchronization declined", "clean both working trees or recreate the remote clone, then rerun devsync sync")
	}
	return nil
}

func initialSyncRiskError(report status.Report) error {
	return apperrors.NewWithRemedy(apperrors.ErrInitialSyncRisk, "initial synchronization requires verification", "ensure local and remote working trees match, or recreate the remote clone from the latest local state, then rerun: devsync sync")
}

func writeInitialSyncWarning(out io.Writer, report status.Report) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Initial synchronization detected.")
	fmt.Fprintln(out, "Local and remote repositories share Git ancestry, but working trees may differ before the first Mutagen baseline.")
	if len(report.Initial.Reasons) > 0 {
		fmt.Fprintln(out, "Risk indicators:")
		for _, reason := range report.Initial.Reasons {
			fmt.Fprintf(out, "  - %s\n", reason)
		}
	}
	fmt.Fprintln(out, "Recommended:")
	fmt.Fprintln(out, "  - clean working trees")
	fmt.Fprintln(out, "  - matching checkouts")
	fmt.Fprintln(out, "  - fresh remote clone before first sync")
}

func ensureWorkspaceConfigured(ctx context.Context, out io.Writer) error {
	ws, err := workspace.Discover(ctx)
	if err != nil {
		return err
	}
	exists, _, err := workspace.HasWorkspaceConfig(ws.Root)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	cfg, err := workspace.ResolveConfig(ws)
	if err != nil {
		return apperrors.NewWithRemedy(apperrors.ErrWorkspaceConfigMissing, "workspace is not configured and convention mapping could not be inferred", "run devsync init --remote-host <host> --remote-path <path> or move the repository under the configured local root")
	}
	if !isInteractive() {
		return apperrors.NewWithRemedy(apperrors.ErrWorkspaceConfigMissing, "workspace is not configured", "run devsync bootstrap --init-workspace")
	}
	fmt.Fprintln(out, "No DevSync workspace configuration found.")
	fmt.Fprintf(out, "Detected Git repository: %s\n", cfg.Workspace.Name)
	fmt.Fprintln(out, "Inferred mapping:")
	fmt.Fprintf(out, "  local:  %s\n", ws.Root)
	fmt.Fprintf(out, "  remote: %s:%s\n", cfg.Remote.Host, cfg.Remote.Path)
	fmt.Fprint(out, "Initialize this workspace? [y/N] ")
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		return apperrors.NewWithRemedy(apperrors.ErrWorkspaceConfigMissing, "workspace initialization declined", "run devsync bootstrap --init-workspace when ready")
	}
	path, err := workspace.WriteLocalOverride(ws.Root, cfg)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Created workspace config: %s\n", path)
	fmt.Fprintln(out, "Next steps: devsync doctor; devsync sync --dry-run")
	return nil
}

func isInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func writePlan(out io.Writer, syncPlan plan.Plan) {
	fmt.Fprintln(out)
	if syncPlan.DryRun {
		fmt.Fprintln(out, "Dry run plan:")
	} else {
		fmt.Fprintln(out, "Plan:")
	}
	for _, op := range syncPlan.Ops {
		mutation := ""
		if op.Mutates {
			mutation = " (mutates)"
		}
		fmt.Fprintf(out, "  - [%s] %s%s\n", op.Kind, op.Description, mutation)
	}
}

func runBootstrap(ctx context.Context, out io.Writer, initWorkspace bool) error {
	fmt.Fprintln(out, "DevSync bootstrap")
	path, created, err := workspace.EnsureGlobalConfig()
	if err != nil {
		return err
	}
	if created {
		fmt.Fprintf(out, "Global config: created %s\n", path)
	} else {
		fmt.Fprintf(out, "Global config: exists %s\n", path)
	}
	tool(out, "git", "Git CLI")
	tool(out, "ssh", "SSH CLI")
	tool(out, "mutagen", "Mutagen CLI")
	ws, err := workspace.Discover(ctx)
	if err != nil {
		fmt.Fprintf(out, "Workspace: not detected (%v)\n", err)
		return nil
	}
	cfg, err := workspace.ResolveConfig(ws)
	if err != nil {
		fmt.Fprintf(out, "Convention mapping: blocked (%v)\n", err)
		fmt.Fprintln(out, "Hint: move the repository under the configured local root or add .devsync.yaml with an explicit remote.path.")
		return nil
	}
	fmt.Fprintf(out, "Workspace: %s\n", cfg.Workspace.Name)
	fmt.Fprintf(out, "Convention: %s -> %s\n", ws.Root, cfg.Remote.Path)
	fmt.Fprintf(out, "Remote node: %s (%s)\n", cfg.Remote.Node, cfg.Remote.Host)
	if initWorkspace {
		path, err := workspace.WriteLocalOverride(ws.Root, cfg)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Workspace override: wrote %s\n", path)
	} else {
		fmt.Fprintln(out, "Workspace override: skipped (use --init-workspace to write .devsync.yaml)")
	}
	if err := runDoctor(ctx, out); err != nil {
		fmt.Fprintf(out, "Bootstrap validation needs attention: %v\n", err)
		return nil
	}
	return nil
}

func runInitRemote(ctx context.Context, out io.Writer, yes bool) error {
	ws, err := workspace.Discover(ctx)
	if err != nil {
		return err
	}
	cfg, err := workspace.ResolveConfig(ws)
	if err != nil {
		return err
	}
	local, err := git.InspectLocal(ctx, ws.Root)
	if err != nil {
		return err
	}
	if local.Branch == "" {
		return apperrors.NewWithRemedy(apperrors.ErrDetachedHead, "local repository is in detached HEAD state", "checkout a branch before initializing the remote workspace")
	}
	runner := devssh.Runner{Target: cfg.Remote.Target}
	if _, err := runner.Run(ctx, "git --version"); err != nil {
		return err
	}
	remoteState := git.ClassifyRemote(ctx, runner, cfg.Remote.Path, "")
	if remoteState.Kind == git.RemoteValidGitRepo {
		return apperrors.NewWithRemedy(apperrors.ErrRemoteRepoInvalid, "remote workspace is already a Git repository", "run devsync doctor/status; init-remote only seeds missing workspaces")
	}
	if remoteState.Kind != git.RemoteMissing && remoteState.Kind != git.RemoteEmptyDir {
		return apperrors.NewWithRemedy(apperrors.ErrRemoteRepoInvalid, fmt.Sprintf("remote workspace is %s", remoteState.Kind), "move or clean the remote path before running init-remote")
	}
	fmt.Fprintln(out, "Preparing remote workspace:")
	fmt.Fprintf(out, "  local:  %s\n", ws.Root)
	fmt.Fprintf(out, "  remote: %s:%s\n", cfg.Remote.Target.String(), cfg.Remote.Path)
	fmt.Fprintln(out, "Operations:")
	fmt.Fprintln(out, "  - create temporary mirror")
	fmt.Fprintln(out, "  - upload mirror")
	fmt.Fprintln(out, "  - create remote working clone")
	fmt.Fprintln(out, "  - validate remote repository")
	fmt.Fprintln(out, "  - clean temporary artifacts")
	if !yes {
		if !isInteractive() {
			return apperrors.NewWithRemedy(apperrors.ErrInitialSyncRisk, "remote initialization requires explicit confirmation", "rerun with devsync init-remote --yes after reviewing the planned operations")
		}
		fmt.Fprint(out, "Proceed? [y/N] ")
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			return apperrors.New(apperrors.ErrInitialSyncRisk, "remote initialization declined")
		}
	}
	tmpDir, err := os.MkdirTemp("", "devsync-mirror-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	localMirror := filepath.Join(tmpDir, "repo.mirror.git")
	if err := runCommand(ctx, ws.Root, "git", "clone", "--mirror", ".", localMirror); err != nil {
		return err
	}
	remoteMirror := "~/tmp/devsync-" + cfg.Workspace.Name + ".mirror.git"
	defer runner.Run(ctx, "rm -rf "+devssh.QuotePath(remoteMirror))
	if _, err := runner.Run(ctx, "mkdir -p ~/tmp && rm -rf "+devssh.QuotePath(remoteMirror)); err != nil {
		return err
	}
	if err := runCommand(ctx, "", "scp", cfg.Remote.Target.SCPArgs(localMirror, "~/tmp/devsync-"+cfg.Workspace.Name+".mirror.git")...); err != nil {
		return err
	}
	if _, err := runner.Run(ctx, "mkdir -p $(dirname "+devssh.QuotePath(cfg.Remote.Path)+") && rm -rf "+devssh.QuotePath(cfg.Remote.Path)+" && git clone "+devssh.QuotePath(remoteMirror)+" "+devssh.QuotePath(cfg.Remote.Path)); err != nil {
		return err
	}
	remote, err := git.InspectRemote(ctx, runner, cfg.Remote.Path)
	if err != nil {
		return err
	}
	if remote.Branch != local.Branch || remote.Head != local.Head || remote.Dirty {
		return apperrors.NewWithRemedy(apperrors.ErrRemoteRepoInvalid, "remote seed validation failed", "inspect the remote repository manually before syncing")
	}
	fmt.Fprintln(out, "Remote workspace initialized successfully.")
	fmt.Fprintln(out, "Next steps: devsync doctor; devsync sync --dry-run; devsync sync")
	return nil
}

func runCommand(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return nil
}

func runDoctor(ctx context.Context, out io.Writer) error {
	fmt.Fprintln(out, "DevSync doctor")
	tool(out, "git", "Git CLI")
	tool(out, "ssh", "SSH CLI")
	tool(out, "mutagen", "Mutagen CLI")
	version(out, "git", "--version", "Local Git version")
	version(out, "mutagen", "version", "Mutagen version")
	disk(out, ".", "Local disk")
	report, err := buildStatus(ctx)
	if err != nil {
		fmt.Fprintf(out, "Workspace: failed\n  %v\n", err)
		return err
	}
	fmt.Fprintf(out, "Workspace: ok (%s)\n", report.Workspace.Root)
	fmt.Fprintf(out, "Config: node=%s host=%s path=%s\n", report.Config.Remote.Node, report.Config.Remote.Host, report.Config.Remote.Path)
	sshRoundTrip(ctx, out, report.Config.Remote.Target)
	remoteVersion(ctx, out, report.Config.Remote.Target, "git --version", "Remote Git version")
	remoteVersion(ctx, out, report.Config.Remote.Target, "df -h "+devssh.QuotePath(report.Config.Remote.Path), "Remote disk")
	lockInspection(out, report.Config.Workspace.Name)
	fmt.Fprintf(out, "Remote repo: ok (%s)\n", report.Remote.Branch)
	fmt.Fprintf(out, "History: local ahead=%d remote ahead=%d known=%v\n", report.Compare.LocalAhead, report.Compare.RemoteAhead, report.Compare.Known)
	fmt.Fprintf(out, "Mutagen: %s exists=%v healthy=%v\n", report.Sync.Status, report.Sync.Exists, report.Sync.Healthy)
	if report.Reconcile.Needed {
		fmt.Fprintf(out, "Reconciliation: needed (%s)\n", report.Action)
		return report.Err
	}
	if !report.Safe {
		fmt.Fprintf(out, "Safety: blocked\n  %v\n", report.Err)
		return report.Err
	}
	fmt.Fprintln(out, "Safety: ok")
	return nil
}

func tool(out io.Writer, binary string, label string) {
	if path, err := exec.LookPath(binary); err == nil {
		fmt.Fprintf(out, "%s: ok (%s)\n", label, path)
		return
	}
	fmt.Fprintf(out, "%s: missing\n", label)
}

func version(out io.Writer, binary string, arg string, label string) {
	args := strings.Fields(arg)
	cmd := exec.Command(binary, args...)
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(out, "%s: unavailable\n", label)
		return
	}
	fmt.Fprintf(out, "%s: %s\n", label, strings.TrimSpace(string(output)))
}

func disk(out io.Writer, path string, label string) {
	cmd := exec.Command("df", "-h", path)
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(out, "%s: unavailable\n", label)
		return
	}
	fmt.Fprintf(out, "%s:\n%s\n", label, strings.TrimSpace(string(output)))
}

func sshRoundTrip(ctx context.Context, out io.Writer, target devssh.Target) {
	start := time.Now()
	_, err := devssh.Runner{Target: target}.Run(ctx, "true")
	if err != nil {
		fmt.Fprintf(out, "SSH round trip: failed (%v)\n", err)
		return
	}
	fmt.Fprintf(out, "SSH round trip: %s\n", time.Since(start).Round(time.Millisecond))
}

func remoteVersion(ctx context.Context, out io.Writer, target devssh.Target, command string, label string) {
	output, err := devssh.Runner{Target: target}.Run(ctx, command)
	if err != nil {
		fmt.Fprintf(out, "%s: unavailable (%v)\n", label, err)
		return
	}
	if len(output) > 400 {
		output = output[:400]
	}
	fmt.Fprintf(out, "%s: %s\n", label, strings.TrimSpace(output))
}

func lockInspection(out io.Writer, workspaceName string) {
	path, err := lock.Path(workspaceName)
	if err != nil {
		fmt.Fprintf(out, "Lockfile: unavailable (%v)\n", err)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(out, "Lockfile: none (%s)\n", path)
			return
		}
		fmt.Fprintf(out, "Lockfile: unreadable (%v)\n", err)
		return
	}
	fmt.Fprintf(out, "Lockfile: present %s\n%s\n", path, strings.TrimSpace(string(bytes.TrimSpace(data))))
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
	remoteRunner := devssh.Runner{Target: cfg.Remote.Target}
	remote, err := git.InspectRemote(ctx, remoteRunner, cfg.Remote.Path)
	if err != nil {
		return status.Report{}, err
	}
	comparison := git.CompareHistories(ctx, ws.Root, local.Head, cfg.Remote.Host, cfg.Remote.Path, remote.Head)
	syncState := mutagen.Inspect(ctx, cfg.Workspace.Name)
	persisted := syncstate.Load(cfg.Workspace.Name)
	reconciliation := mutagen.Reconcile(ws, cfg, syncState)

	return status.Evaluate(ws, cfg, local, remote, comparison, syncState, persisted, reconciliation), nil
}
