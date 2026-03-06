package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellToolExecutesCommandInWorkspace(t *testing.T) {
	workspace := t.TempDir()
	subdir := filepath.Join(workspace, "nested")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	tool := NewShellTool(workspace)
	result, err := tool.Execute(context.Background(), map[string]any{
		"command":    "printf hello",
		"cwd":        "nested",
		"timeoutSec": 2,
	})
	if err != nil {
		t.Fatalf("execute shell tool: %v", err)
	}

	data := result.Data.(map[string]any)
	if data["stdout"] != "hello" {
		t.Fatalf("expected stdout hello, got %#v", data["stdout"])
	}
	if data["stderr"] != "" {
		t.Fatalf("expected empty stderr, got %#v", data["stderr"])
	}
	if data["exitCode"] != 0 {
		t.Fatalf("expected exitCode 0, got %#v", data["exitCode"])
	}
	if data["cwd"] != subdir {
		t.Fatalf("expected cwd %q, got %#v", subdir, data["cwd"])
	}
}

func TestShellToolRejectsCWDOutsideWorkspace(t *testing.T) {
	tool := NewShellTool(t.TempDir())
	_, err := tool.Execute(context.Background(), map[string]any{
		"command": "pwd",
		"cwd":     "..",
	})
	if err == nil {
		t.Fatal("expected cwd validation error")
	}
	if !strings.Contains(err.Error(), "inside workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShellToolReturnsNonZeroExitCodeWithoutError(t *testing.T) {
	tool := NewShellTool(t.TempDir())
	result, err := tool.Execute(context.Background(), map[string]any{
		"command":    "echo boom >&2; exit 7",
		"timeoutSec": 2,
	})
	if err != nil {
		t.Fatalf("expected structured failure result, got err=%v", err)
	}

	data := result.Data.(map[string]any)
	if data["exitCode"] != 7 {
		t.Fatalf("expected exitCode 7, got %#v", data["exitCode"])
	}
	if !strings.Contains(data["stderr"].(string), "boom") {
		t.Fatalf("expected stderr boom, got %#v", data["stderr"])
	}
}

func TestShellToolTimesOut(t *testing.T) {
	tool := NewShellTool(t.TempDir())
	result, err := tool.Execute(context.Background(), map[string]any{
		"command":    "sleep 2",
		"timeoutSec": 1,
	})
	if err != nil {
		t.Fatalf("expected timeout result, got err=%v", err)
	}

	data := result.Data.(map[string]any)
	if data["timedOut"] != true {
		t.Fatalf("expected timedOut true, got %#v", data["timedOut"])
	}
	if data["exitCode"] != -1 {
		t.Fatalf("expected exitCode -1 on timeout, got %#v", data["exitCode"])
	}
}
