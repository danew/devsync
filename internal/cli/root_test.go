package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danew/devsync/internal/apperrors"
	"github.com/danew/devsync/internal/git"
	"github.com/danew/devsync/internal/logging"
	"github.com/danew/devsync/internal/mutagen"
	"github.com/danew/devsync/internal/plan"
	devssh "github.com/danew/devsync/internal/ssh"
	"github.com/danew/devsync/internal/status"
	"github.com/danew/devsync/internal/workspace"
)

func TestRootCommandRegistersDoctorAndDryRun(t *testing.T) {
	root := newRootCommand()
	for _, command := range []string{"doctor", "bootstrap", "version", "session", "init-remote", "attach", "detach", "forward"} {
		if _, _, err := root.Find([]string{command}); err != nil {
			t.Fatalf("%s command missing: %v", command, err)
		}
	}
	sync, _, err := root.Find([]string{"sync"})
	if err != nil {
		t.Fatal(err)
	}
	if sync.Flags().Lookup("dry-run") == nil {
		t.Fatal("sync --dry-run flag missing")
	}
}

func TestRunBootstrapCreatesGlobalConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var out bytes.Buffer
	if err := runBootstrap(t.Context(), &out, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".config", "devsync", "config.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected global config at %s: %v", path, err)
	}
	if !strings.Contains(out.String(), "Global config: created") {
		t.Fatalf("expected bootstrap output to mention created config, got:\n%s", out.String())
	}
}

func TestWritePlanDryRunDoesNotMutate(t *testing.T) {
	report := status.Report{
		Workspace: workspace.Workspace{Root: "/local"},
		Config: workspace.Config{
			Workspace: workspace.WorkspaceIdentity{Name: "steel-api"},
			Remote:    workspace.RemoteConfig{Host: "core-dev", Path: "~/workspace/work/steel-api", Target: devssh.ParseTarget("core-dev")},
		},
		Local:   git.State{Branch: "main"},
		Compare: git.Comparison{Known: true, LocalAhead: 1},
		Sync:    mutagen.State{SessionName: "devsync-steel-api", Exists: false},
	}
	var out bytes.Buffer
	writePlan(&out, plan.FromReport(report, true))
	text := out.String()
	for _, expected := range []string{"Dry run plan:", "manual Git history synchronization required", "create Mutagen session", "verify post-flush health"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected plan to contain %q, got:\n%s", expected, text)
		}
	}
}

func TestInitialSyncRiskFailsClosedWhenNonInteractive(t *testing.T) {
	report := status.Report{Initial: status.InitialSync{Pending: true, Risky: true, Reasons: []string{"local working tree is dirty"}}}

	err := confirmInitialSync(&bytes.Buffer{}, report)
	if !apperrors.Is(err, apperrors.ErrInitialSyncRisk) {
		t.Fatalf("expected ErrInitialSyncRisk, got %v", err)
	}
}

func TestWriteInitialSyncWarningIncludesGuidance(t *testing.T) {
	report := status.Report{Initial: status.InitialSync{Pending: true, Risky: true, Reasons: []string{"remote working tree is dirty"}}}
	var out bytes.Buffer
	writeInitialSyncWarning(&out, report)
	text := out.String()
	for _, expected := range []string{"Initial synchronization detected", "remote working tree is dirty", "fresh remote clone"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected warning to contain %q, got:\n%s", expected, text)
		}
	}
}

func TestRunOneShotMutagenCreatesFlushesAndPauses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	runner := &fakeMutagenRunner{}
	report := lifecycleReport()
	var out bytes.Buffer

	if err := runOneShotMutagen(t.Context(), &out, runner, report, logging.New(false, false, nil)); err != nil {
		t.Fatal(err)
	}

	runner.assertCommands(t, []string{
		"sync list --long",
		"sync create --name devsync-steel-api --ignore .git /local/steel-api core-dev:~/workspace/work/steel-api",
		"sync list --long",
		"sync flush devsync-steel-api",
		"sync list --long",
		"sync pause devsync-steel-api",
	})
	if !strings.Contains(out.String(), "pausing one-shot session") {
		t.Fatalf("expected one-shot pause output, got:\n%s", out.String())
	}
}

func TestRunOneShotMutagenPausesAfterFlushFailure(t *testing.T) {
	runner := &fakeMutagenRunner{failFlush: true}
	report := lifecycleReport()
	var out bytes.Buffer

	err := runOneShotMutagen(t.Context(), &out, runner, report, logging.New(false, false, nil))
	if err == nil {
		t.Fatal("expected flush failure")
	}
	if !runner.saw("sync pause devsync-steel-api") {
		t.Fatalf("expected deferred pause after flush failure, got %#v", runner.commands)
	}
}

func TestRunAttachMutagenResumesAndLeavesSessionActive(t *testing.T) {
	runner := &fakeMutagenRunner{exists: true, paused: true}
	report := lifecycleReport()
	report.Sync = mutagen.State{SessionName: "devsync-steel-api", Exists: true, Paused: true, Healthy: true}
	var out bytes.Buffer

	if err := runAttachMutagen(t.Context(), &out, runner, report); err != nil {
		t.Fatal(err)
	}
	if !runner.saw("sync resume devsync-steel-api") {
		t.Fatalf("expected attach to resume session, got %#v", runner.commands)
	}
	if runner.saw("sync pause devsync-steel-api") {
		t.Fatalf("attach must not pause continuous session, got %#v", runner.commands)
	}
}

func TestRunDetachMutagenPausesActiveSession(t *testing.T) {
	runner := &fakeMutagenRunner{exists: true}
	report := lifecycleReport()
	report.Sync = mutagen.State{SessionName: "devsync-steel-api", Exists: true, Active: true, Healthy: true}
	var out bytes.Buffer

	if err := runDetachMutagen(t.Context(), &out, runner, report); err != nil {
		t.Fatal(err)
	}
	if !runner.saw("sync pause devsync-steel-api") {
		t.Fatalf("expected detach to pause session, got %#v", runner.commands)
	}
}

func TestRunForwardWithConfigStartsConfiguredForwards(t *testing.T) {
	runner := &fakeForwardRunner{}
	cfg := workspace.Config{
		Remote: workspace.RemoteConfig{Target: devssh.Target{User: "dev", Host: "100.72.16.64", Port: "22"}},
		Forward: workspace.ForwardConfig{Ports: []workspace.PortForward{
			{LocalPort: "3000", RemoteHost: "127.0.0.1", RemotePort: "3000"},
			{LocalPort: "5173", RemoteHost: "127.0.0.1", RemotePort: "5173"},
		}},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	if err := runForwardWithConfig(t.Context(), &out, &errOut, strings.NewReader(""), runner, cfg); err != nil {
		t.Fatal(err)
	}

	if len(runner.forwards) != 2 {
		t.Fatalf("forwards = %#v", runner.forwards)
	}
	if got := runner.forwards[0].RenderSSH(); got != "3000:127.0.0.1:3000" {
		t.Fatalf("first forward = %q", got)
	}
	if !strings.Contains(out.String(), "Forwarding SSH ports via dev@100.72.16.64:22") {
		t.Fatalf("expected forwarding output, got:\n%s", out.String())
	}
}

func TestRunForwardWithConfigRequiresConfiguredForwards(t *testing.T) {
	err := runForwardWithConfig(t.Context(), &bytes.Buffer{}, &bytes.Buffer{}, strings.NewReader(""), &fakeForwardRunner{}, workspace.Config{})
	if err == nil || !strings.Contains(err.Error(), "no port forwards configured") {
		t.Fatalf("expected missing forwards error, got %v", err)
	}
}

func lifecycleReport() status.Report {
	return status.Report{
		Workspace: workspace.Workspace{Root: "/local/steel-api"},
		Config: workspace.Config{
			Workspace: workspace.WorkspaceIdentity{Name: "steel-api"},
			Remote:    workspace.RemoteConfig{Host: "core-dev", Path: "~/workspace/work/steel-api", Target: devssh.ParseTarget("core-dev")},
		},
		Sync: mutagen.State{SessionName: "devsync-steel-api", Healthy: true},
		Safe: true,
	}
}

type fakeForwardRunner struct {
	forwards []devssh.LocalForward
}

func (r *fakeForwardRunner) Forward(ctx context.Context, forwards []devssh.LocalForward, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	r.forwards = append([]devssh.LocalForward{}, forwards...)
	return nil
}

type fakeMutagenRunner struct {
	commands  []string
	exists    bool
	paused    bool
	failFlush bool
}

func (r *fakeMutagenRunner) Run(ctx context.Context, args ...string) (string, error) {
	command := strings.Join(args, " ")
	r.commands = append(r.commands, command)
	switch {
	case command == "sync list --long":
		if !r.exists {
			return "", nil
		}
		status := "Watching for changes"
		if r.paused {
			status = "Paused"
		}
		return fmt.Sprintf("Name: devsync-steel-api\nStatus: %s\nAlpha: /local/steel-api\nBeta: core-dev:~/workspace/work/steel-api", status), nil
	case strings.HasPrefix(command, "sync create "):
		r.exists = true
		r.paused = false
		return "", nil
	case command == "sync resume devsync-steel-api":
		r.paused = false
		return "", nil
	case command == "sync pause devsync-steel-api":
		r.paused = true
		return "", nil
	case command == "sync flush devsync-steel-api":
		if r.failFlush {
			return "", fmt.Errorf("flush failed")
		}
		return "", nil
	default:
		return "", fmt.Errorf("unexpected mutagen command: %s", command)
	}
}

func (r *fakeMutagenRunner) saw(command string) bool {
	for _, candidate := range r.commands {
		if candidate == command {
			return true
		}
	}
	return false
}

func (r *fakeMutagenRunner) assertCommands(t *testing.T, expected []string) {
	t.Helper()
	if strings.Join(r.commands, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("commands = %#v, want %#v", r.commands, expected)
	}
}
