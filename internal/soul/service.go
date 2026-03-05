package soul

import (
	"os"
	"path/filepath"
)

type Service struct {
	workspace string
}

func NewService(workspace string) *Service {
	return &Service{workspace: workspace}
}

func (s *Service) Load() string {
	b, err := os.ReadFile(filepath.Join(s.workspace, "SOUL.md"))
	if err != nil {
		return ""
	}
	return string(b)
}
