package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultShellTimeout = 10 * time.Second
	maxShellTimeout     = 60 * time.Second
	maxShellOutputBytes = 16 * 1024
)

type ShellTool struct {
	workspaceDir   string
	defaultTimeout time.Duration
	maxTimeout     time.Duration
	maxOutputBytes int
}

func NewShellTool(workspaceDir string) *ShellTool {
	return &ShellTool{
		workspaceDir:   strings.TrimSpace(workspaceDir),
		defaultTimeout: defaultShellTimeout,
		maxTimeout:     maxShellTimeout,
		maxOutputBytes: maxShellOutputBytes,
	}
}

func (t *ShellTool) Name() string {
	return "shell"
}

func (t *ShellTool) Metadata() Metadata {
	return Metadata{
		Name:        t.Name(),
		Description: "Runs a short shell command from the workspace with bounded output and timeout.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to run.",
				},
				"cwd": map[string]any{
					"type":        "string",
					"description": "Optional working directory inside the workspace.",
				},
				"timeoutSec": map[string]any{
					"type":        "integer",
					"description": "Optional timeout in seconds (1-60).",
					"minimum":     1,
					"maximum":     60,
				},
			},
			"required": []string{"command"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
				"cwd":     map[string]any{"type": "string"},
				"exitCode": map[string]any{
					"type": "integer",
				},
				"stdout": map[string]any{"type": "string"},
				"stderr": map[string]any{"type": "string"},
				"timedOut": map[string]any{
					"type": "boolean",
				},
				"truncated": map[string]any{
					"type": "boolean",
				},
			},
			"required": []string{"command", "cwd", "exitCode", "stdout", "stderr", "timedOut", "truncated"},
		},
	}
}

func (t *ShellTool) Execute(ctx context.Context, input map[string]any) (Result, error) {
	command := strings.TrimSpace(asString(input["command"]))
	if command == "" {
		return Result{}, errors.New("shell: command is required")
	}

	cwd, err := t.resolveCWD(asString(input["cwd"]))
	if err != nil {
		return Result{}, err
	}
	timeout, err := t.resolveTimeout(input["timeoutSec"])
	if err != nil {
		return Result{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "/bin/sh", "-lc", command)
	cmd.Dir = cwd

	stdout := newLimitedBuffer(t.maxOutputBytes)
	stderr := newLimitedBuffer(t.maxOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		switch {
		case timedOut:
			exitCode = -1
		case errors.As(runErr, &exitErr):
			exitCode = exitErr.ExitCode()
		default:
			return Result{}, fmt.Errorf("shell: run command: %w", runErr)
		}
	}

	return Result{Data: map[string]any{
		"command":   command,
		"cwd":       cwd,
		"exitCode":  exitCode,
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"timedOut":  timedOut,
		"truncated": stdout.Truncated() || stderr.Truncated(),
	}}, nil
}

func (t *ShellTool) resolveCWD(raw string) (string, error) {
	base := strings.TrimSpace(t.workspaceDir)
	if base == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("shell: resolve workspace dir: %w", err)
		}
		base = wd
	}

	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("shell: resolve workspace dir: %w", err)
	}

	target := baseAbs
	if value := strings.TrimSpace(raw); value != "" {
		if filepath.IsAbs(value) {
			target = filepath.Clean(value)
		} else {
			target = filepath.Join(baseAbs, value)
		}
	}

	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("shell: resolve cwd: %w", err)
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return "", fmt.Errorf("shell: resolve cwd: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("shell: cwd must stay inside workspace")
	}

	info, err := os.Stat(targetAbs)
	if err != nil {
		return "", fmt.Errorf("shell: stat cwd: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("shell: cwd must be a directory")
	}
	return targetAbs, nil
}

func (t *ShellTool) resolveTimeout(raw any) (time.Duration, error) {
	if raw == nil {
		return t.defaultTimeout, nil
	}
	sec, err := asInt(raw)
	if err != nil {
		return 0, errors.New("shell: timeoutSec must be an integer")
	}
	if sec <= 0 {
		return 0, errors.New("shell: timeoutSec must be >= 1")
	}
	timeout := time.Duration(sec) * time.Second
	if timeout > t.maxTimeout {
		return 0, fmt.Errorf("shell: timeoutSec must be <= %d", int(t.maxTimeout/time.Second))
	}
	return timeout, nil
}

func asString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		return ""
	}
}

func asInt(v any) (int, error) {
	switch value := v.(type) {
	case int:
		return value, nil
	case int8:
		return int(value), nil
	case int16:
		return int(value), nil
	case int32:
		return int(value), nil
	case int64:
		return int(value), nil
	case float64:
		if value != float64(int(value)) {
			return 0, errors.New("not an integer")
		}
		return int(value), nil
	case string:
		return strconv.Atoi(strings.TrimSpace(value))
	default:
		return 0, errors.New("unsupported type")
	}
}

type limitedBuffer struct {
	buf       bytes.Buffer
	remaining int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	if limit <= 0 {
		limit = maxShellOutputBytes
	}
	return &limitedBuffer{remaining: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	written := len(p)
	if b.remaining <= 0 {
		b.truncated = true
		return written, nil
	}
	if len(p) > b.remaining {
		_, _ = b.buf.Write(p[:b.remaining])
		b.remaining = 0
		b.truncated = true
		return written, nil
	}
	_, _ = b.buf.Write(p)
	b.remaining -= len(p)
	return written, nil
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}

func (b *limitedBuffer) Truncated() bool {
	return b.truncated
}
