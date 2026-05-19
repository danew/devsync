package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/danew/devsync/internal/apperrors"
	devssh "github.com/danew/devsync/internal/ssh"
)

type State struct {
	Branch       string
	Head         string
	Dirty        bool
	DirtyEntries []string
}

type Comparison struct {
	LocalAhead  int
	RemoteAhead int
	Known       bool
	Err         error
}

type RemoteWorkspaceKind string

const (
	RemoteMissing        RemoteWorkspaceKind = "missing"
	RemoteSSHFailure     RemoteWorkspaceKind = "ssh-failure"
	RemoteEmptyDir       RemoteWorkspaceKind = "empty-directory"
	RemoteNonGitDir      RemoteWorkspaceKind = "non-git-directory"
	RemoteValidGitRepo   RemoteWorkspaceKind = "valid-git-repo"
	RemoteBranchMismatch RemoteWorkspaceKind = "branch-mismatch"
	RemoteDiverged       RemoteWorkspaceKind = "diverged"
)

type RemoteWorkspaceState struct {
	Kind    RemoteWorkspaceKind
	Branch  string
	Head    string
	Dirty   bool
	Error   error
	Command string
	Target  devssh.Target
}

func InspectLocal(ctx context.Context, root string) (State, error) {
	branch, err := runGit(ctx, root, "branch", "--show-current")
	if err != nil {
		return State{}, err
	}
	head, err := runGit(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return State{}, err
	}
	dirty, err := runGit(ctx, root, "status", "--porcelain")
	if err != nil {
		return State{}, err
	}
	entries := splitLines(dirty)
	return State{Branch: branch, Head: head, Dirty: len(entries) > 0, DirtyEntries: entries}, nil
}

func InspectRemote(ctx context.Context, runner devssh.Runner, path string) (State, error) {
	classification := ClassifyRemote(ctx, runner, path, "")
	if classification.Error != nil {
		return State{}, classification.Error
	}
	if classification.Kind != RemoteValidGitRepo {
		return State{}, remoteStateError(classification.Kind, path)
	}
	return State{Branch: classification.Branch, Head: classification.Head, Dirty: classification.Dirty}, nil
}

func ClassifyRemote(ctx context.Context, runner devssh.Runner, path string, expectedBranch string) RemoteWorkspaceState {
	remotePath := devssh.QuotePath(path)
	traceRemoteClassification("start", RemoteWorkspaceKind(""), runner, path, "")
	if result, err := runner.RunRaw(ctx, "test -d "+remotePath); err != nil {
		state := RemoteWorkspaceState{Kind: RemoteSSHFailure, Error: err, Command: result.Command, Target: result.Target}
		traceRemoteClassification("ssh-failed", state.Kind, runner, path, state.Command)
		return state
	} else if result.ExitCode != 0 {
		state := RemoteWorkspaceState{Kind: RemoteMissing, Command: result.Command, Target: result.Target}
		traceRemoteClassification("classified", state.Kind, runner, path, state.Command)
		return state
	}
	if result, err := runner.RunRaw(ctx, "find "+remotePath+" -mindepth 1 -maxdepth 1 | head -n 1"); err != nil {
		state := RemoteWorkspaceState{Kind: RemoteNonGitDir, Error: err, Command: result.Command, Target: result.Target}
		traceRemoteClassification("ssh-failed", state.Kind, runner, path, state.Command)
		return state
	} else if result.ExitCode == 0 && strings.TrimSpace(result.Stdout) == "" {
		state := RemoteWorkspaceState{Kind: RemoteEmptyDir, Command: result.Command, Target: result.Target}
		traceRemoteClassification("classified", state.Kind, runner, path, state.Command)
		return state
	}
	gitCheck := "git -C " + remotePath + " rev-parse --is-inside-work-tree"
	if result, err := runner.RunRaw(ctx, gitCheck); err != nil {
		state := RemoteWorkspaceState{Kind: RemoteNonGitDir, Error: err, Command: result.Command, Target: result.Target}
		traceRemoteClassification("ssh-failed", state.Kind, runner, path, state.Command)
		return state
	} else if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "true" {
		state := RemoteWorkspaceState{Kind: RemoteNonGitDir, Command: result.Command, Target: result.Target}
		traceRemoteClassification("classified", state.Kind, runner, path, state.Command)
		return state
	}
	branch, err := runner.Run(ctx, "git -C "+remotePath+" branch --show-current")
	if err != nil {
		return RemoteWorkspaceState{Kind: RemoteNonGitDir, Error: err}
	}
	if expectedBranch != "" && branch != expectedBranch {
		state := RemoteWorkspaceState{Kind: RemoteBranchMismatch, Branch: branch}
		traceRemoteClassification("classified", state.Kind, runner, path, "git branch --show-current")
		return state
	}
	head, err := runner.Run(ctx, "git -C "+remotePath+" rev-parse HEAD")
	if err != nil {
		return RemoteWorkspaceState{Kind: RemoteNonGitDir, Error: err}
	}
	dirty, err := runner.Run(ctx, "git -C "+remotePath+" status --porcelain")
	if err != nil {
		return RemoteWorkspaceState{Kind: RemoteNonGitDir, Error: err}
	}
	dirtyEntries := splitLines(dirty)
	state := RemoteWorkspaceState{Kind: RemoteValidGitRepo, Branch: branch, Head: head, Dirty: len(dirtyEntries) > 0}
	traceRemoteClassification("classified", state.Kind, runner, path, "git status --porcelain")
	return state
}

func remoteStateError(kind RemoteWorkspaceKind, path string) error {
	switch kind {
	case RemoteMissing:
		return apperrors.NewWithRemedy(apperrors.ErrRemoteRepoMissing, fmt.Sprintf("remote workspace path does not exist: %s", path), "verify remote.ssh user/host and remote.path, run the reproduce command from trace output, or run devsync init-remote")
	case RemoteEmptyDir:
		return apperrors.NewWithRemedy(apperrors.ErrRemoteRepoMissing, fmt.Sprintf("remote workspace path is empty: %s", path), "run devsync init-remote to seed the repository")
	default:
		return apperrors.NewWithRemedy(apperrors.ErrRemoteRepoInvalid, fmt.Sprintf("remote path is not a Git work tree: %s", path), "verify the remote path points at an initialized repository or run devsync init-remote")
	}
}

func RemoteReproduceCommand(target devssh.Target, command string) string {
	return "ssh " + target.String() + " " + devssh.Quote(command)
}

func traceRemoteClassification(phase string, kind RemoteWorkspaceKind, runner devssh.Runner, path string, command string) {
	if os.Getenv("DEVSYNC_TRACE") == "" {
		return
	}
	fields := []string{
		"level=trace",
		"event=remote.classification",
		"phase=" + quoteLog(phase),
		"path=" + quoteLog(path),
	}
	if kind != "" {
		fields = append(fields, "kind="+quoteLog(string(kind)))
	}
	if command != "" {
		fields = append(fields, "command="+quoteLog(command))
	}
	fmt.Fprintln(os.Stderr, strings.Join(fields, " "))
}

func quoteLog(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func CompareHistories(ctx context.Context, localRoot, localHead, remoteHost, remotePath, remoteHead string) Comparison {
	return CompareHistoriesWithRunner(ctx, localRoot, localHead, devssh.Runner{Host: remoteHost}, remotePath, remoteHead)
}

func CompareHistoriesWithRunner(ctx context.Context, localRoot, localHead string, runner devssh.Runner, remotePath, remoteHead string) Comparison {
	if localHead == remoteHead {
		return Comparison{Known: true}
	}
	if comparison, err := compareLocal(ctx, localRoot, localHead, remoteHead); err == nil {
		return comparison
	}
	command := "git -C " + devssh.QuotePath(remotePath) + " rev-list --left-right --count " + devssh.Quote(localHead+"..."+remoteHead)
	out, err := runner.Run(ctx, command)
	if err != nil {
		return Comparison{Known: false, Err: apperrors.New(apperrors.ErrHistoryUnknown, "unable to compare histories; neither repository can see both HEAD commits")}
	}
	comparison, err := parseRevListCount(out)
	if err != nil {
		return Comparison{Known: false, Err: err}
	}
	return comparison
}

func PushBranch(ctx context.Context, root, remoteHost, remotePath, branch string) error {
	if err := ValidateMutationReadiness(ctx, root, branch); err != nil {
		return err
	}
	remote := GitRemoteURL(remoteHost, remotePath)
	_, err := runGit(ctx, root, "push", remote, branch+":"+branch)
	if err != nil {
		return fmt.Errorf("push local commits to remote branch: %w", err)
	}
	return nil
}

func PullBranchFastForward(ctx context.Context, root, remoteHost, remotePath, branch string) error {
	if err := ValidateMutationReadiness(ctx, root, branch); err != nil {
		return err
	}
	remote := GitRemoteURL(remoteHost, remotePath)
	_, err := runGit(ctx, root, "pull", "--ff-only", remote, branch)
	if err != nil {
		return fmt.Errorf("pull remote commits with fast-forward only: %w", err)
	}
	return nil
}

func ValidateMutationReadiness(ctx context.Context, root, expectedBranch string) error {
	state, err := InspectLocal(ctx, root)
	if err != nil {
		return err
	}
	if state.Branch == "" {
		return apperrors.NewWithRemedy(apperrors.ErrDetachedHead, "local repository is in detached HEAD state", "checkout a branch before running devsync sync")
	}
	if state.Branch != expectedBranch {
		return apperrors.NewWithRemedy(apperrors.ErrBranchMismatch, fmt.Sprintf("current branch changed during sync: expected %s, got %s", expectedBranch, state.Branch), "rerun devsync status and retry after branch state is stable")
	}
	return nil
}

func CheckRemoteBranchVisible(ctx context.Context, runner devssh.Runner, path, expectedBranch string) error {
	state, err := InspectRemote(ctx, runner, path)
	if err != nil {
		return err
	}
	if state.Branch == "" {
		return apperrors.NewWithRemedy(apperrors.ErrDetachedHead, "remote repository is in detached HEAD state", "checkout the matching branch on the remote repository")
	}
	if state.Branch != expectedBranch {
		return apperrors.NewWithRemedy(apperrors.ErrBranchMismatch, fmt.Sprintf("remote branch changed during sync: expected %s, got %s", expectedBranch, state.Branch), "rerun devsync status and retry after branch state is stable")
	}
	return nil
}

func GitRemoteURL(host, path string) string {
	if path == "" {
		return host
	}
	return host + ":" + path
}

func compareLocal(ctx context.Context, root, localHead, remoteHead string) (Comparison, error) {
	out, err := runGit(ctx, root, "rev-list", "--left-right", "--count", localHead+"..."+remoteHead)
	if err != nil {
		return Comparison{}, err
	}
	return parseRevListCount(out)
}

func runGit(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func parseRevListCount(output string) (Comparison, error) {
	fields := strings.Fields(output)
	if len(fields) != 2 {
		return Comparison{}, fmt.Errorf("unexpected rev-list count output: %q", output)
	}
	left, err := strconv.Atoi(fields[0])
	if err != nil {
		return Comparison{}, fmt.Errorf("parse local ahead count: %w", err)
	}
	right, err := strconv.Atoi(fields[1])
	if err != nil {
		return Comparison{}, fmt.Errorf("parse remote ahead count: %w", err)
	}
	return Comparison{LocalAhead: left, RemoteAhead: right, Known: true}, nil
}

func splitLines(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(value, "\n"), "\n")
}
