package git

import (
	"bytes"
	"context"
	"fmt"
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
	RemoteEmptyDir       RemoteWorkspaceKind = "empty-directory"
	RemoteNonGitDir      RemoteWorkspaceKind = "non-git-directory"
	RemoteValidGitRepo   RemoteWorkspaceKind = "valid-git-repo"
	RemoteBranchMismatch RemoteWorkspaceKind = "branch-mismatch"
	RemoteDiverged       RemoteWorkspaceKind = "diverged"
)

type RemoteWorkspaceState struct {
	Kind   RemoteWorkspaceKind
	Branch string
	Head   string
	Dirty  bool
	Error  error
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
	if _, err := runner.Run(ctx, "test -d "+remotePath); err != nil {
		return RemoteWorkspaceState{Kind: RemoteMissing}
	}
	firstEntry, err := runner.Run(ctx, "find "+remotePath+" -mindepth 1 -maxdepth 1 | head -n 1")
	if err == nil && strings.TrimSpace(firstEntry) == "" {
		return RemoteWorkspaceState{Kind: RemoteEmptyDir}
	}
	inside, err := runner.Run(ctx, "cd "+remotePath+" && git rev-parse --is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		return RemoteWorkspaceState{Kind: RemoteNonGitDir}
	}
	branch, err := runner.Run(ctx, "cd "+remotePath+" && git branch --show-current")
	if err != nil {
		return RemoteWorkspaceState{Kind: RemoteNonGitDir, Error: err}
	}
	if expectedBranch != "" && branch != expectedBranch {
		return RemoteWorkspaceState{Kind: RemoteBranchMismatch, Branch: branch}
	}
	head, err := runner.Run(ctx, "cd "+remotePath+" && git rev-parse HEAD")
	if err != nil {
		return RemoteWorkspaceState{Kind: RemoteNonGitDir, Error: err}
	}
	dirty, err := runner.Run(ctx, "cd "+remotePath+" && git status --porcelain")
	if err != nil {
		return RemoteWorkspaceState{Kind: RemoteNonGitDir, Error: err}
	}
	dirtyEntries := splitLines(dirty)
	return RemoteWorkspaceState{Kind: RemoteValidGitRepo, Branch: branch, Head: head, Dirty: len(dirtyEntries) > 0}
}

func remoteStateError(kind RemoteWorkspaceKind, path string) error {
	switch kind {
	case RemoteMissing:
		return apperrors.NewWithRemedy(apperrors.ErrRemoteRepoMissing, fmt.Sprintf("remote workspace path does not exist: %s", path), "verify remote.ssh user/host and remote.path, or run devsync init-remote")
	case RemoteEmptyDir:
		return apperrors.NewWithRemedy(apperrors.ErrRemoteRepoMissing, fmt.Sprintf("remote workspace path is empty: %s", path), "run devsync init-remote to seed the repository")
	default:
		return apperrors.NewWithRemedy(apperrors.ErrRemoteRepoInvalid, fmt.Sprintf("remote path is not a Git work tree: %s", path), "verify the remote path points at an initialized repository or run devsync init-remote")
	}
}

func CompareHistories(ctx context.Context, localRoot, localHead, remoteHost, remotePath, remoteHead string) Comparison {
	if localHead == remoteHead {
		return Comparison{Known: true}
	}
	if comparison, err := compareLocal(ctx, localRoot, localHead, remoteHead); err == nil {
		return comparison
	}
	runner := devssh.Runner{Host: remoteHost}
	command := "cd " + devssh.QuotePath(remotePath) + " && git rev-list --left-right --count " + devssh.Quote(localHead+"..."+remoteHead)
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
