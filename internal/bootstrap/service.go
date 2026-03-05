package bootstrap

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ni6io/nisix/internal/domain"
	"github.com/ni6io/nisix/internal/workspace"
)

type ContextBundle struct {
	Identity      domain.AgentIdentity
	SoulText      string
	ProjectPrompt string
}

type Service struct {
	workspace string
	log       *slog.Logger
}

func NewService(workspaceDir string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		workspace: workspaceDir,
		log:       logger,
	}
}

func (s *Service) LoadContext(_ string, _ domain.InboundMessage) (ContextBundle, error) {
	id := loadIdentity(s.workspace)
	soul := readText(filepath.Join(s.workspace, "SOUL.md"))
	sections := []struct {
		file string
		body string
	}{
		{"AGENTS.md", readText(filepath.Join(s.workspace, "AGENTS.md"))},
		{"TOOLS.md", readText(filepath.Join(s.workspace, "TOOLS.md"))},
		{"USER.md", readText(filepath.Join(s.workspace, "USER.md"))},
		{"BOOTSTRAP.md", readText(filepath.Join(s.workspace, "BOOTSTRAP.md"))},
	}
	var b strings.Builder
	for _, section := range sections {
		if strings.TrimSpace(section.body) == "" {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("# Project Context\n\n")
		}
		b.WriteString("## ")
		b.WriteString(section.file)
		b.WriteString("\n")
		b.WriteString(section.body)
		b.WriteString("\n\n")
	}
	return ContextBundle{
		Identity:      id,
		SoulText:      soul,
		ProjectPrompt: strings.TrimSpace(b.String()),
	}, nil
}

func (s *Service) Status() (workspace.Status, error) {
	return workspace.GetStatus(s.workspace)
}

func (s *Service) Complete(removeBootstrap bool) (workspace.Status, error) {
	st, err := workspace.CompleteOnboarding(s.workspace, removeBootstrap)
	if err == nil {
		s.log.Info("bootstrap.completed", "workspace", s.workspace, "bootstrapRemoved", !st.BootstrapExists)
	}
	return st, err
}

func loadIdentity(workspaceDir string) domain.AgentIdentity {
	id := domain.AgentIdentity{Name: "Assistant", Avatar: "A"}
	path := filepath.Join(workspaceDir, "IDENTITY.md")
	f, err := os.Open(path)
	if err != nil {
		return id
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "name:"):
			id.Name = strings.TrimSpace(line[len("name:"):])
		case strings.HasPrefix(lower, "avatar:"):
			id.Avatar = strings.TrimSpace(line[len("avatar:"):])
		case strings.HasPrefix(lower, "emoji:"):
			id.Emoji = strings.TrimSpace(line[len("emoji:"):])
		}
	}
	return id
}

func readText(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (s *Service) Workspace() string {
	return s.workspace
}

func (s *Service) String() string {
	return fmt.Sprintf("bootstrap.Service(%s)", s.workspace)
}
