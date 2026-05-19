package ssh

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/danew/devsync/internal/apperrors"
)

type Runner struct {
	Host string
}

func (r Runner) Run(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "ssh", r.Host, command)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", apperrors.Wrap(apperrors.ErrRemoteUnreachable, fmt.Sprintf("ssh %s %q failed", r.Host, command), fmt.Errorf("%s", strings.TrimSpace(stderr.String())))
	}
	return strings.TrimSpace(string(out)), nil
}

func Quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func QuotePath(path string) string {
	if path == "~" {
		return "$HOME"
	}
	if strings.HasPrefix(path, "~/") {
		return "$HOME/" + Quote(strings.TrimPrefix(path, "~/"))
	}
	return Quote(path)
}
