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
	remotePath := devssh.QuotePath(path)
	branch, err := runner.Run(ctx, "cd "+remotePath+" && git branch --show-current")
	if err != nil {
		return State{}, err
	}
	head, err := runner.Run(ctx, "cd "+remotePath+" && git rev-parse HEAD")
	if err != nil {
		return State{}, err
	}
	dirty, err := runner.Run(ctx, "cd "+remotePath+" && git status --porcelain")
	if err != nil {
		return State{}, err
	}
	entries := splitLines(dirty)
	return State{Branch: branch, Head: head, Dirty: len(entries) > 0, DirtyEntries: entries}, nil
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
	remote := GitRemoteURL(remoteHost, remotePath)
	_, err := runGit(ctx, root, "push", remote, branch+":"+branch)
	if err != nil {
		return fmt.Errorf("push local commits to remote branch: %w", err)
	}
	return nil
}

func PullBranchFastForward(ctx context.Context, root, remoteHost, remotePath, branch string) error {
	remote := GitRemoteURL(remoteHost, remotePath)
	_, err := runGit(ctx, root, "pull", "--ff-only", remote, branch)
	if err != nil {
		return fmt.Errorf("pull remote commits with fast-forward only: %w", err)
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
