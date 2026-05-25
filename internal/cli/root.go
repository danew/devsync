package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
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
	root.AddCommand(newAttachCommand())
	root.AddCommand(newDetachCommand())
	root.AddCommand(newForwardCommand())

	return root
}

type portForwardRunner interface {
	Forward(ctx context.Context, forwards []devssh.LocalForward, stdin io.Reader, stdout io.Writer, stderr io.Writer) error
}

func newForwardCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "forward",
		Short: "Forward configured ports over SSH",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runForward(cmd.Context(), os.Stdout, os.Stderr)
		},
	}
}

func newAttachCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "attach",
		Short: "Attach continuous synchronization intentionally",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAttach(cmd.Context(), os.Stdout, mutagen.CLIRunner{})
		},
	}
}

func newDetachCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "detach",
		Short: "Pause continuous synchronization",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDetach(cmd.Context(), os.Stdout, mutagen.CLIRunner{})
		},
	}
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
		Short: "Run one-shot workspace synchronization",
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
		syncState, err := mutagen.EnsureSession(cmd.Context(), runner, report.Workspace, report.Config)
		if err != nil {
			return err
		}
		return mutagen.Pause(cmd.Context(), runner, syncState.SessionName)
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

func runSync(ctx context.Context, out io.Writer, dryRun bool, logger logging.Logger) (err error) {
	syncCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer func() {
		if syncCtx.Err() != nil {
			logger.Debug("sync.interrupted", map[string]string{"signal": "interrupt"})
			if err != nil {
				err = apperrors.NewWithRemedy(apperrors.ErrInterrupted, "sync interrupted", "lockfile released; rerun devsync status before retrying")
			}
		}
	}()
	logger.Debug("sync.start", map[string]string{"dry_run": fmt.Sprintf("%v", dryRun)})
	if err := ensureWorkspaceConfigured(syncCtx, out); err != nil {
		return err
	}
	report, err := buildStatus(syncCtx)
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
	defer func() {
		reason := "normal"
		if syncCtx.Err() != nil {
			reason = "interrupt"
		}
		_ = workspaceLock.Release()
		logger.Debug("lock.released", map[string]string{"workspace": report.Config.Workspace.Name, "reason": reason})
	}()
	logger.Debug("sync.lock_acquired", map[string]string{"workspace": report.Config.Workspace.Name})

	if err := confirmInitialSync(out, report); err != nil {
		return err
	}

	if err := runOneShotMutagen(syncCtx, out, mutagen.CLIRunner{}, report, logger); err != nil {
		return err
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Sync complete.")
	return nil
}

func runOneShotMutagen(ctx context.Context, out io.Writer, runner mutagen.Runner, report status.Report, logger logging.Logger) error {
	started := time.Now()
	syncState, err := mutagen.EnsureSession(ctx, runner, report.Workspace, report.Config)
	if err != nil {
		return err
	}
	oneShotPaused := false
	defer func() {
		if !oneShotPaused {
			pauseOneShotSession(syncState.SessionName, runner, logger)
		}
	}()
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
	fmt.Fprintf(out, "Mutagen: pausing one-shot session %s\n", syncState.SessionName)
	if err := mutagen.Pause(ctx, runner, syncState.SessionName); err != nil {
		return err
	}
	oneShotPaused = true
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
	return nil
}

func pauseOneShotSession(sessionName string, runner mutagen.Runner, logger logging.Logger) {
	if sessionName == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mutagen.Pause(ctx, runner, sessionName); err != nil {
		logger.Debug("sync.pause_after_exit_failed", map[string]string{"session": sessionName, "error": err.Error()})
		return
	}
	logger.Debug("sync.pause_after_exit", map[string]string{"session": sessionName})
}

func runAttach(ctx context.Context, out io.Writer, runner mutagen.Runner) error {
	report, err := buildStatus(ctx)
	if err != nil {
		return err
	}
	ui.WriteStatus(out, report)
	return runAttachMutagen(ctx, out, runner, report)
}

func runAttachMutagen(ctx context.Context, out io.Writer, runner mutagen.Runner, report status.Report) error {
	if !report.Safe {
		return report.Err
	}
	syncState, err := mutagen.EnsureSession(ctx, runner, report.Workspace, report.Config)
	if err != nil {
		return err
	}
	if syncState.Paused {
		fmt.Fprintf(out, "Mutagen: resuming continuous session %s\n", syncState.SessionName)
		if err := mutagen.Resume(ctx, runner, syncState.SessionName); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "Mutagen: continuous session active %s\n", syncState.SessionName)
	}
	fmt.Fprintln(out, "Attached. Continuous synchronization is active until devsync detach.")
	return nil
}

func runDetach(ctx context.Context, out io.Writer, runner mutagen.Runner) error {
	report, err := buildStatus(ctx)
	if err != nil {
		return err
	}
	return runDetachMutagen(ctx, out, runner, report)
}

func runDetachMutagen(ctx context.Context, out io.Writer, runner mutagen.Runner, report status.Report) error {
	if !report.Sync.Exists {
		fmt.Fprintf(out, "Mutagen: no session to detach (%s)\n", report.Sync.SessionName)
		return nil
	}
	if report.Sync.Paused {
		fmt.Fprintf(out, "Mutagen: session already detached %s\n", report.Sync.SessionName)
		return nil
	}
	fmt.Fprintf(out, "Mutagen: pausing continuous session %s\n", report.Sync.SessionName)
	if err := mutagen.Pause(ctx, runner, report.Sync.SessionName); err != nil {
		return err
	}
	fmt.Fprintln(out, "Detached. Background synchronization is stopped.")
	return nil
}

func runForward(ctx context.Context, out io.Writer, errOut io.Writer) error {
	ws, err := workspace.Discover(ctx)
	if err != nil {
		return err
	}
	cfg, err := workspace.ResolveConfig(ws)
	if err != nil {
		return err
	}
	return runForwardWithConfig(ctx, out, errOut, os.Stdin, devssh.Runner{Target: cfg.Remote.Target}, cfg)
}

func runForwardWithConfig(ctx context.Context, out io.Writer, errOut io.Writer, stdin io.Reader, runner portForwardRunner, cfg workspace.Config) error {
	forwards := configuredForwards(cfg.Forward)
	if len(forwards) == 0 {
		return fmt.Errorf("no port forwards configured; add forward.ports to .devsync.yaml")
	}
	forwardCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(out, "Forwarding SSH ports via %s\n", cfg.Remote.Target.RenderSSH())
	for _, forward := range forwards {
		fmt.Fprintf(out, "  - %s\n", forward.RenderSSH())
	}
	fmt.Fprintln(out, "Press Ctrl-C to stop forwarding.")
	if err := runner.Forward(forwardCtx, forwards, stdin, out, errOut); err != nil {
		if forwardCtx.Err() != nil {
			fmt.Fprintln(out, "Forwarding stopped.")
			return nil
		}
		return err
	}
	return nil
}

func configuredForwards(cfg workspace.ForwardConfig) []devssh.LocalForward {
	forwards := []devssh.LocalForward{}
	for _, port := range cfg.Ports {
		forwards = append(forwards, devssh.LocalForward{
			LocalHost:  port.LocalHost,
			LocalPort:  port.LocalPort,
			RemoteHost: port.RemoteHost,
			RemotePort: port.RemotePort,
		})
	}
	return forwards
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
	canonicalOrigin, err := git.CaptureOrigin(ctx, ws.Root)
	if err != nil {
		return err
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
	fmt.Fprintln(out, "Canonical Git remotes to preserve:")
	if canonicalOrigin == nil {
		fmt.Fprintln(out, "  warning: no origin remote found; remote repository will be left without origin")
	} else {
		fmt.Fprintf(out, "  origin -> %s\n", canonicalOrigin.FetchURL)
		if canonicalOrigin.PushURL != "" && canonicalOrigin.PushURL != canonicalOrigin.FetchURL {
			fmt.Fprintf(out, "  origin push -> %s\n", canonicalOrigin.PushURL)
		}
	}
	fmt.Fprintln(out, "Operations:")
	fmt.Fprintln(out, "  - create temporary mirror")
	fmt.Fprintln(out, "  - upload mirror")
	fmt.Fprintln(out, "  - create remote working clone")
	fmt.Fprintln(out, "  - restore canonical Git remotes")
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
	remoteTopology, err := git.RestoreRemoteOrigin(ctx, runner, cfg.Remote.Path, canonicalOrigin, remoteMirror)
	if err != nil {
		return err
	}
	remote, err := git.InspectRemote(ctx, runner, cfg.Remote.Path)
	if err != nil {
		return err
	}
	if remote.Branch != local.Branch || remote.Head != local.Head || remote.Dirty {
		return apperrors.NewWithRemedy(apperrors.ErrRemoteRepoInvalid, "remote seed validation failed", "inspect the remote repository manually before syncing")
	}
	if strings.TrimSpace(remoteTopology) == "" {
		fmt.Fprintln(out, "Remote Git remotes: none")
	} else {
		fmt.Fprintln(out, "Remote Git remotes:")
		for _, line := range strings.Split(remoteTopology, "\n") {
			fmt.Fprintf(out, "  %s\n", line)
		}
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
	fmt.Fprintf(out, "Config: node=%s target=%s path=%s\n", report.Config.Remote.Node, report.Config.Remote.Target.RenderSSH(), report.Config.Remote.Path)
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
	comparison := git.CompareHistoriesWithRunner(ctx, ws.Root, local.Head, remoteRunner, cfg.Remote.Path, remote.Head)
	syncState := mutagen.Inspect(ctx, cfg.Workspace.Name)
	persisted := syncstate.Load(cfg.Workspace.Name)
	reconciliation := mutagen.Reconcile(ws, cfg, syncState)

	return status.Evaluate(ws, cfg, local, remote, comparison, syncState, persisted, reconciliation), nil
}
