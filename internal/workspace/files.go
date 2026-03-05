package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var BootstrapFiles = []string{
	"AGENTS.md",
	"IDENTITY.md",
	"SOUL.md",
	"TOOLS.md",
	"USER.md",
	"MEMORY.md",
	"POLICY.md",
	"ROUTING.md",
	"MODELS.md",
	"BOOTSTRAP.md",
	filepath.Join("memory", "README.md"),
	filepath.Join("skills", "README.md"),
}

const (
	stateDirName  = ".nisix"
	stateFileName = "workspace_state.json"
)

type Options struct {
	BootstrapFromTemplates bool
	TemplateDir            string
}

type OnboardingState struct {
	Version               int    `json:"version"`
	SeededAt              string `json:"seededAt,omitempty"`
	OnboardingCompletedAt string `json:"onboardingCompletedAt,omitempty"`
}

type FileStatus struct {
	File   string `json:"file"`
	Exists bool   `json:"exists"`
}

type Status struct {
	Seeded              bool         `json:"seeded"`
	OnboardingCompleted bool         `json:"onboardingCompleted"`
	BootstrapExists     bool         `json:"bootstrapExists"`
	Files               []FileStatus `json:"files"`
}

func DefaultTemplateDir(root string) string {
	return filepath.Join(filepath.Dir(root), "templates")
}

func EnsureLayout(root string, opts Options) error {
	if opts.TemplateDir == "" {
		opts.TemplateDir = DefaultTemplateDir(root)
	}
	state, err := readState(root)
	if err != nil {
		return err
	}
	workspaceIsNew, err := isWorkspaceNew(root)
	if err != nil {
		return err
	}

	seeded := false
	for _, rel := range BootstrapFiles {
		if rel == "BOOTSTRAP.md" && state.OnboardingCompletedAt != "" {
			continue
		}
		if rel == "BOOTSTRAP.md" && !workspaceIsNew && state.SeededAt == "" {
			continue
		}
		wrote, err := ensureFile(root, opts.TemplateDir, rel, opts.BootstrapFromTemplates)
		if err != nil {
			return err
		}
		seeded = seeded || wrote
	}
	if seeded && state.SeededAt == "" {
		state.SeededAt = time.Now().UTC().Format(time.RFC3339)
		if err := writeState(root, state); err != nil {
			return err
		}
	}
	return nil
}

func GetStatus(root string) (Status, error) {
	state, err := readState(root)
	if err != nil {
		return Status{}, err
	}
	out := Status{
		Seeded:              state.SeededAt != "",
		OnboardingCompleted: state.OnboardingCompletedAt != "",
		Files:               make([]FileStatus, 0, len(BootstrapFiles)),
	}
	for _, rel := range BootstrapFiles {
		_, statErr := os.Stat(filepath.Join(root, rel))
		exists := statErr == nil
		out.Files = append(out.Files, FileStatus{File: rel, Exists: exists})
		if rel == "BOOTSTRAP.md" {
			out.BootstrapExists = exists
		}
	}
	return out, nil
}

func CompleteOnboarding(root string, removeBootstrap bool) (Status, error) {
	state, err := readState(root)
	if err != nil {
		return Status{}, err
	}
	if state.SeededAt == "" {
		state.SeededAt = time.Now().UTC().Format(time.RFC3339)
	}
	if state.OnboardingCompletedAt == "" {
		state.OnboardingCompletedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := writeState(root, state); err != nil {
		return Status{}, err
	}
	if removeBootstrap {
		_ = os.Remove(filepath.Join(root, "BOOTSTRAP.md"))
	}
	return GetStatus(root)
}

func readState(root string) (OnboardingState, error) {
	path := statePath(root)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return OnboardingState{Version: 1}, nil
		}
		return OnboardingState{}, fmt.Errorf("workspace: read state: %w", err)
	}
	var state OnboardingState
	if err := json.Unmarshal(b, &state); err != nil {
		return OnboardingState{}, fmt.Errorf("workspace: parse state: %w", err)
	}
	if state.Version == 0 {
		state.Version = 1
	}
	return state, nil
}

func writeState(root string, state OnboardingState) error {
	state.Version = 1
	if err := os.MkdirAll(filepath.Dir(statePath(root)), 0o755); err != nil {
		return fmt.Errorf("workspace: mkdir state dir: %w", err)
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := statePath(root) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, statePath(root))
}

func statePath(root string) string {
	return filepath.Join(root, stateDirName, stateFileName)
}

func isWorkspaceNew(root string) (bool, error) {
	for _, rel := range BootstrapFiles {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			return false, nil
		}
	}
	if _, err := os.Stat(filepath.Join(root, "memory")); err == nil {
		return false, nil
	}
	return true, nil
}

func ensureFile(root, templateDir, rel string, useTemplate bool) (bool, error) {
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false, fmt.Errorf("workspace: mkdir %s: %w", p, err)
	}
	if _, err := os.Stat(p); err == nil {
		return false, nil
	}
	content := []byte("\n")
	if useTemplate {
		tb, err := os.ReadFile(filepath.Join(templateDir, rel))
		if err == nil {
			content = tb
			if len(content) == 0 || content[len(content)-1] != '\n' {
				content = append(content, '\n')
			}
		}
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		return false, fmt.Errorf("workspace: write %s: %w", p, err)
	}
	return true, nil
}
