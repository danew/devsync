package syncstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/danew/devsync/internal/workspace"
)

type State struct {
	Workspace       string    `json:"workspace"`
	SessionName     string    `json:"session_name"`
	LastFlushAt     time.Time `json:"last_flush_at"`
	LastDirection   string    `json:"last_direction"`
	LastRemoteHost  string    `json:"last_remote_host"`
	LastRemotePath  string    `json:"last_remote_path"`
	LastLocalRoot   string    `json:"last_local_root"`
	LastMutagenMode string    `json:"last_mutagen_mode"`
}

func Load(workspaceName string) State {
	path, err := path(workspaceName)
	if err != nil {
		return State{Workspace: workspaceName}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return State{Workspace: workspaceName}
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{Workspace: workspaceName}
	}
	return state
}

func Save(state State) error {
	path, err := path(state.Workspace)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create sync state directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func path(workspaceName string) (string, error) {
	dir, err := workspace.DevSyncConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state", workspaceName+".json"), nil
}
