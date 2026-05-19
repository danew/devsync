package lock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/danew/devsync/internal/apperrors"
	"github.com/danew/devsync/internal/workspace"
)

const staleAfter = 2 * time.Hour

type Metadata struct {
	Workspace string    `json:"workspace"`
	PID       int       `json:"pid"`
	Command   string    `json:"command"`
	CreatedAt time.Time `json:"created_at"`
}

type Lock struct {
	path string
}

func Acquire(workspaceName, command string) (*Lock, error) {
	path, err := Path(workspaceName)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	if stale(path) {
		_ = os.Remove(path)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, apperrors.NewWithRemedy(apperrors.ErrWorkspaceLockHeld, fmt.Sprintf("devsync lock already exists: %s", path), "inspect the lock file; remove it only if no devsync process is running")
		}
		return nil, fmt.Errorf("create lock file: %w", err)
	}
	defer file.Close()
	metadata := Metadata{Workspace: workspaceName, PID: os.Getpid(), Command: command, CreatedAt: time.Now().UTC()}
	if err := json.NewEncoder(file).Encode(metadata); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("write lock metadata: %w", err)
	}
	return &Lock{path: path}, nil
}

func (l *Lock) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	return os.Remove(l.path)
}

func Path(workspaceName string) (string, error) {
	dir, err := workspace.DevSyncConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "locks", workspaceName+".lock"), nil
}

func stale(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) > staleAfter
}
