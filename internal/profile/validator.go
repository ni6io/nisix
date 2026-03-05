package profile

import (
	"fmt"
	"path/filepath"
	"strings"
)

func defaultAllowedFiles() []string {
	return []string{"IDENTITY.md", "SOUL.md", "USER.md", "TOOLS.md", "AGENTS.md", "MEMORY.md"}
}

func normalizeMode(raw string, fallback string) UpdateMode {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		v = strings.ToLower(strings.TrimSpace(fallback))
	}
	switch v {
	case string(UpdateModeAppend):
		return UpdateModeAppend
	case string(UpdateModePatch):
		return UpdateModePatch
	default:
		return UpdateModeReplace
	}
}

func normalizeFile(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return ""
	}
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	return name
}

func validateFile(name string, allow map[string]struct{}) error {
	if name == "" {
		return fmt.Errorf("FORBIDDEN_FILE")
	}
	if strings.Contains(name, "/") || strings.Contains(name, `\`) {
		return fmt.Errorf("FORBIDDEN_FILE")
	}
	if _, ok := allow[strings.ToUpper(name)]; !ok {
		return fmt.Errorf("FORBIDDEN_FILE")
	}
	return nil
}

func validateSize(content string, maxFileBytes int) error {
	if maxFileBytes <= 0 {
		maxFileBytes = 262144
	}
	if len([]byte(content)) > maxFileBytes {
		return fmt.Errorf("FILE_TOO_LARGE")
	}
	return nil
}
