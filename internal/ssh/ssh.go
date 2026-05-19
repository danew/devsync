package ssh

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/danew/devsync/internal/apperrors"
)

type Runner struct {
	Host   string
	Target Target
}

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Target   Target
	Command  string
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
	result, err := r.RunRaw(ctx, command)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("ssh %s %q exited %d: %s", result.Target.String(), command, result.ExitCode, result.Stderr)
	}
	return result.Stdout, nil
}

func (r Runner) RunRaw(ctx context.Context, command string) (Result, error) {
	target := r.target()
	cmd := exec.CommandContext(ctx, "ssh", target.sshArgs(command)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	traceSSH(target, command, "start", "", "", -1)
	err := cmd.Run()
	result := Result{Stdout: strings.TrimSpace(stdout.String()), Stderr: strings.TrimSpace(stderr.String()), ExitCode: 0, Target: target, Command: command}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			traceSSH(target, command, "exit", result.Stdout, result.Stderr, result.ExitCode)
			if result.ExitCode == 255 {
				return result, apperrors.NewWithRemedy(apperrors.ErrRemoteUnreachable, fmt.Sprintf("SSH connection failed for %s while running: %s\nstderr: %s", target.String(), command, result.Stderr), "verify remote.ssh user/host/port and reproduce with: ssh "+target.String()+" "+Quote(command))
			}
			return result, nil
		}
		traceSSH(target, command, "transport_error", result.Stdout, result.Stderr, -1)
		return result, apperrors.Wrap(apperrors.ErrRemoteUnreachable, fmt.Sprintf("ssh %s %q failed", target.String(), command), fmt.Errorf("%s", strings.TrimSpace(stderr.String())))
	}
	traceSSH(target, command, "exit", result.Stdout, result.Stderr, 0)
	return result, nil
}

func traceSSH(target Target, command string, phase string, stdout string, stderr string, exitCode int) {
	if os.Getenv("DEVSYNC_TRACE") == "" {
		return
	}
	fields := []string{
		"level=trace",
		"event=ssh.command",
		"phase=" + quoteLog(phase),
		"target=" + quoteLog(target.String()),
		"user=" + quoteLog(target.User),
		"host=" + quoteLog(target.Host),
		"port=" + quoteLog(target.Port),
		"alias=" + quoteLog(target.Alias),
		"command=" + quoteLog(command),
	}
	if exitCode >= 0 {
		fields = append(fields, "exit_code="+strconv.Itoa(exitCode))
	}
	if stdout != "" {
		fields = append(fields, "stdout="+quoteLog(stdout))
	}
	if stderr != "" {
		fields = append(fields, "stderr="+quoteLog(stderr))
	}
	fmt.Fprintln(os.Stderr, strings.Join(fields, " "))
}

func quoteLog(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
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
