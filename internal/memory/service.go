package memory

import (
	"os"
	"path/filepath"
	"strings"
)

type Service struct {
	workspace string
}

func NewService(workspace string) *Service {
	return &Service{workspace: workspace}
}

func (s *Service) Search(query string) ([]string, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, nil
	}
	paths := []string{
		filepath.Join(s.workspace, "MEMORY.md"),
	}
	files, _ := filepath.Glob(filepath.Join(s.workspace, "memory", "*.md"))
	paths = append(paths, files...)

	out := make([]string, 0)
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		text := strings.ToLower(string(b))
		if strings.Contains(text, query) {
			out = append(out, p)
		}
	}
	return out, nil
}
