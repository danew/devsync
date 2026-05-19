package mutagen

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/danew/devsync/internal/apperrors"
	"github.com/danew/devsync/internal/workspace"
)

type State struct {
	SessionName   string
	Available     bool
	Exists        bool
	Active        bool
	Paused        bool
	Healthy       bool
	Problems      []string
	LastDirection string
	Message       string
}

type Runner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

type CLIRunner struct{}

func (CLIRunner) Run(ctx context.Context, args ...string) (string, error) {
	if _, err := exec.LookPath("mutagen"); err != nil {
		return "", apperrors.New(apperrors.ErrMutagenUnavailable, "mutagen CLI not found in PATH")
	}
	cmd := exec.CommandContext(ctx, "mutagen", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("mutagen %s: %s", strings.Join(args, " "), message)
	}
	return strings.TrimSpace(string(out)), nil
}

func SessionName(workspaceName string) string {
	return "devsync-" + workspaceName
}

func Inspect(ctx context.Context, workspaceName string) State {
	state, err := InspectWithRunner(ctx, CLIRunner{}, workspaceName)
	if err != nil {
		state.Message = err.Error()
	}
	return state
}

func InspectWithRunner(ctx context.Context, runner Runner, workspaceName string) (State, error) {
	name := SessionName(workspaceName)
	out, err := runner.Run(ctx, "sync", "list")
	if err != nil {
		return State{SessionName: name, Available: !apperrors.Is(err, apperrors.ErrMutagenUnavailable), Message: err.Error()}, err
	}
	if !strings.Contains(out, name) {
		return State{SessionName: name, Available: true, Exists: false, Message: "no session found"}, nil
	}
	return ParseListOutput(name, sessionBlock(name, out)), nil
}

func EnsureSession(ctx context.Context, runner Runner, ws workspace.Workspace, cfg workspace.Config) (State, error) {
	state, err := InspectWithRunner(ctx, runner, cfg.Workspace.Name)
	if err != nil {
		return state, err
	}
	if !state.Exists {
		if _, err := runner.Run(ctx, CreateArgs(ws.Root, cfg)...); err != nil {
			return state, fmt.Errorf("create mutagen session %s: %w", SessionName(cfg.Workspace.Name), err)
		}
		state, err = InspectWithRunner(ctx, runner, cfg.Workspace.Name)
		if err != nil {
			return state, err
		}
		if !state.Exists {
			return state, apperrors.New(apperrors.ErrMutagenUnhealthy, "mutagen session creation did not produce an inspectable session")
		}
	}
	return state, nil
}

func Resume(ctx context.Context, runner Runner, sessionName string) error {
	_, err := runner.Run(ctx, "sync", "resume", sessionName)
	if err != nil {
		return fmt.Errorf("resume mutagen session %s: %w", sessionName, err)
	}
	return nil
}

func Flush(ctx context.Context, runner Runner, sessionName string) error {
	_, err := runner.Run(ctx, "sync", "flush", sessionName)
	if err != nil {
		return fmt.Errorf("flush mutagen session %s: %w", sessionName, err)
	}
	return nil
}

func CreateArgs(localRoot string, cfg workspace.Config) []string {
	args := []string{"sync", "create", "--name", SessionName(cfg.Workspace.Name)}
	for _, ignore := range normalizedIgnores(cfg.Sync.Ignores) {
		args = append(args, "--ignore", ignore)
	}
	args = append(args, localRoot, cfg.Remote.Host+":"+cfg.Remote.Path)
	return args
}

func ParseListOutput(sessionName string, output string) State {
	lower := strings.ToLower(output)
	state := State{SessionName: sessionName, Available: true, Exists: strings.TrimSpace(output) != ""}
	if !state.Exists {
		state.Message = "no session found"
		return state
	}
	state.Paused = strings.Contains(lower, "paused")
	state.Active = !state.Paused
	state.Healthy = !strings.Contains(lower, "problem") && !strings.Contains(lower, "conflict") && !strings.Contains(lower, "error")
	state.Problems = problemLines(output)
	state.LastDirection = directionFromOutput(lower)
	state.Message = fmt.Sprintf("session %s detected", sessionName)
	return state
}

func normalizedIgnores(ignores []string) []string {
	seen := map[string]bool{}
	result := []string{".git"}
	seen[".git"] = true
	for _, ignore := range ignores {
		ignore = strings.TrimSpace(ignore)
		if ignore == "" || seen[ignore] {
			continue
		}
		seen[ignore] = true
		result = append(result, ignore)
	}
	return result
}

func problemLines(output string) []string {
	var problems []string
	for _, line := range strings.Split(output, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "problem") || strings.Contains(lower, "conflict") || strings.Contains(lower, "error") {
			problems = append(problems, strings.TrimSpace(line))
		}
	}
	return problems
}

func directionFromOutput(output string) string {
	switch {
	case strings.Contains(output, "alpha") && strings.Contains(output, "beta"):
		return "bidirectional"
	case strings.Contains(output, "local") && strings.Contains(output, "remote"):
		return "bidirectional"
	default:
		return "unknown"
	}
}

func sessionBlock(sessionName, output string) string {
	lines := strings.Split(output, "\n")
	var block []string
	collecting := false
	for _, line := range lines {
		if strings.Contains(line, sessionName) {
			collecting = true
		}
		if collecting {
			block = append(block, line)
		}
	}
	if len(block) == 0 {
		return output
	}
	return strings.Join(block, "\n")
}
