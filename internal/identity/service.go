package identity

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/ni6io/nisix/internal/domain"
)

type Service struct {
	workspace string
}

func NewService(workspace string) *Service {
	return &Service{workspace: workspace}
}

func (s *Service) Load() domain.AgentIdentity {
	id := domain.AgentIdentity{Name: "Assistant", Avatar: "A"}
	path := filepath.Join(s.workspace, "IDENTITY.md")
	f, err := os.Open(path)
	if err != nil {
		return id
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(strings.ToLower(line), "name:") {
			id.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		}
		if strings.HasPrefix(strings.ToLower(line), "avatar:") {
			id.Avatar = strings.TrimSpace(strings.TrimPrefix(line, "avatar:"))
		}
		if strings.HasPrefix(strings.ToLower(line), "emoji:") {
			id.Emoji = strings.TrimSpace(strings.TrimPrefix(line, "emoji:"))
		}
	}
	return id
}
