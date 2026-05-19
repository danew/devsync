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
	Host   string
	Target Target
}

type Target struct {
	User  string
	Host  string
	Port  string
	Alias string
}

func ParseTarget(value string) Target {
	target := Target{}
	if strings.Contains(value, "@") {
		parts := strings.SplitN(value, "@", 2)
		target.User = parts[0]
		value = parts[1]
	}
	if strings.Contains(value, ":") && !strings.Contains(value, "]") {
		parts := strings.SplitN(value, ":", 2)
		target.Host = parts[0]
		target.Port = parts[1]
	} else {
		target.Host = value
	}
	if target.User == "" && target.Port == "" {
		target.Alias = target.Host
	}
	return target
}

func (t Target) String() string {
	if t.Alias != "" {
		return t.Alias
	}
	host := t.Host
	if t.User != "" {
		host = t.User + "@" + host
	}
	if t.Port != "" {
		host += ":" + t.Port
	}
	return host
}

func (t Target) sshArgs(command string) []string {
	host := t.Host
	if t.Alias != "" {
		host = t.Alias
	} else if t.User != "" {
		host = t.User + "@" + host
	}
	args := []string{}
	if t.Port != "" {
		args = append(args, "-p", t.Port)
	}
	return append(args, host, command)
}

func (t Target) SCPDestination(path string) string {
	host := t.Host
	if t.Alias != "" {
		host = t.Alias
	} else if t.User != "" {
		host = t.User + "@" + host
	}
	return host + ":" + path
}

func (t Target) SCPArgs(source string, destination string) []string {
	args := []string{"-r"}
	if t.Port != "" {
		args = append(args, "-P", t.Port)
	}
	return append(args, source, t.SCPDestination(destination))
}

func (r Runner) target() Target {
	if r.Target.Host != "" || r.Target.Alias != "" {
		return r.Target
	}
	return ParseTarget(r.Host)
}

func (r Runner) Run(ctx context.Context, command string) (string, error) {
	target := r.target()
	cmd := exec.CommandContext(ctx, "ssh", target.sshArgs(command)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", apperrors.Wrap(apperrors.ErrRemoteUnreachable, fmt.Sprintf("ssh %s %q failed", target.String(), command), fmt.Errorf("%s", strings.TrimSpace(stderr.String())))
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
