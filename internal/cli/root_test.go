package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danew/devsync/internal/apperrors"
	"github.com/danew/devsync/internal/git"
	"github.com/danew/devsync/internal/mutagen"
	"github.com/danew/devsync/internal/plan"
	"github.com/danew/devsync/internal/status"
	"github.com/danew/devsync/internal/workspace"
)

func TestRootCommandRegistersDoctorAndDryRun(t *testing.T) {
	root := newRootCommand()
	for _, command := range []string{"doctor", "bootstrap", "version", "session"} {
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
			Remote:    workspace.RemoteConfig{Host: "core-dev", Path: "~/workspace/work/steel-api"},
		},
		Local:   git.State{Branch: "main"},
		Compare: git.Comparison{Known: true, LocalAhead: 1},
		Sync:    mutagen.State{SessionName: "devsync-steel-api", Exists: false},
	}
	var out bytes.Buffer
	writePlan(&out, plan.FromReport(report, true))
	text := out.String()
	for _, expected := range []string{"Dry run plan:", "push main", "create Mutagen session", "verify post-flush health"} {
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
