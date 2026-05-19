package workspace

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type Workspace struct {
	Name string
	Root string
}

func Discover(ctx context.Context) (Workspace, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Workspace{}, fmt.Errorf("not inside a Git repository: %s", strings.TrimSpace(stderr.String()))
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return Workspace{}, fmt.Errorf("git returned an empty repository root")
	}
	return Workspace{Name: filepath.Base(root), Root: root}, nil
}
