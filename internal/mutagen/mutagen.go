package mutagen

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type State struct {
	SessionName string
	Available   bool
	Active      bool
	Healthy     bool
	Message     string
}

func Inspect(ctx context.Context, workspaceName string) State {
	sessionName := "devsync-" + workspaceName
	cmd := exec.CommandContext(ctx, "mutagen", "sync", "list", sessionName)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = "no active session found"
		}
		return State{SessionName: sessionName, Available: false, Message: message}
	}
	text := string(out)
	healthy := !strings.Contains(strings.ToLower(text), "problem")
	return State{SessionName: sessionName, Available: true, Active: true, Healthy: healthy, Message: fmt.Sprintf("session %s detected", sessionName)}
}
