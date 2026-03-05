package skills

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

type Service struct {
	cfg Config
	log *slog.Logger

	mu      sync.RWMutex
	loaded  []Skill
	byName  map[string]Skill
	lastErr error
}

func NewService(cfg Config, logger *slog.Logger) *Service {
	if cfg.MaxInjected <= 0 {
		cfg.MaxInjected = 1
	}
	if cfg.MaxBodyChars <= 0 {
		cfg.MaxBodyChars = 4000
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{cfg: cfg, log: logger, byName: make(map[string]Skill)}
}

func (s *Service) LoadAll(workspace string) ([]Skill, error) {
	root := filepath.Join(workspace, "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			s.setLoaded(nil, nil)
			s.log.Info("skills.discovered", "count", 0, "workspace", workspace)
			return nil, nil
		}
		s.setLoaded(nil, err)
		return nil, err
	}

	dirs := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(root, entry.Name()))
		}
	}
	slices.Sort(dirs)

	loaded := make([]Skill, 0, len(dirs))
	seen := make(map[string]struct{})
	allowset := make(map[string]struct{}, len(s.cfg.Allowlist))
	for _, name := range s.cfg.Allowlist {
		allowset[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}

	for _, dir := range dirs {
		path := filepath.Join(dir, "SKILL.md")
		skill, parseErr := s.loadOne(path)
		skill.Path = path
		if skill.Name == "" {
			skill.Name = filepath.Base(dir)
		}

		reason := ""
		enabled := true
		nameKey := strings.ToLower(strings.TrimSpace(skill.Name))

		switch {
		case !s.cfg.Enabled:
			enabled = false
			reason = "global_disabled"
		case parseErr != nil:
			enabled = false
			reason = "parse_error"
		case nameKey == "":
			enabled = false
			reason = "parse_error"
		case hasEntryDisabled(s.cfg.Entries, nameKey):
			enabled = false
			reason = "entry_disabled"
		case len(allowset) > 0:
			if _, ok := allowset[nameKey]; !ok {
				enabled = false
				reason = "not_allowlisted"
			}
		}

		if enabled {
			if _, ok := seen[nameKey]; ok {
				enabled = false
				reason = "duplicate_name"
			} else {
				seen[nameKey] = struct{}{}
			}
		}

		skill.Enabled = enabled
		skill.Reason = reason
		loaded = append(loaded, skill)
		if !enabled {
			s.log.Info("skills.blocked", "name", skill.Name, "reason", reason, "path", skill.Path)
		}
	}

	s.setLoaded(loaded, nil)
	s.log.Info("skills.discovered", "count", len(loaded), "workspace", workspace)
	return loaded, nil
}

func (s *Service) SelectForMessage(message string, explicit []string) ([]Skill, error) {
	s.mu.RLock()
	loaded := append([]Skill(nil), s.loaded...)
	s.mu.RUnlock()
	enabled := make([]Skill, 0)
	for _, sk := range loaded {
		if sk.Enabled {
			enabled = append(enabled, sk)
		}
	}

	selected := selectExplicitSkills(explicit, enabled, s.cfg.MaxInjected)
	if len(selected) > 0 {
		s.log.Info("skills.selected", "mode", "explicit", "count", len(selected), "names", skillNames(selected))
		return selected, nil
	}
	if len(explicit) > 0 {
		return nil, nil
	}
	if !s.cfg.AutoMatch {
		return nil, nil
	}

	selected = selectAutoMatchedSkills(message, enabled, s.cfg.MaxInjected)
	if len(selected) > 0 {
		s.log.Info("skills.selected", "mode", "auto", "count", len(selected), "names", skillNames(selected))
	}
	return selected, nil
}

func (s *Service) LoadedSkills() []Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Skill(nil), s.loaded...)
}

func (s *Service) FindByName(name string) (Skill, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sk, ok := s.byName[strings.ToLower(strings.TrimSpace(name))]
	return sk, ok
}

func (s *Service) loadOne(path string) (Skill, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	meta, body, err := parseSkillMarkdown(string(b))
	if err != nil {
		return Skill{}, err
	}
	if s.cfg.MaxBodyChars > 0 && len(body) > s.cfg.MaxBodyChars {
		body = body[:s.cfg.MaxBodyChars] + "\n\n...[truncated]"
	}
	return Skill{
		Name:        strings.TrimSpace(meta.Name),
		Description: strings.TrimSpace(meta.Description),
		Body:        strings.TrimSpace(body),
	}, nil
}

func (s *Service) setLoaded(skills []Skill, loadErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loaded = skills
	s.lastErr = loadErr
	s.byName = make(map[string]Skill, len(skills))
	for _, sk := range skills {
		key := strings.ToLower(strings.TrimSpace(sk.Name))
		if key == "" {
			continue
		}
		if _, exists := s.byName[key]; exists {
			continue
		}
		s.byName[key] = sk
	}
}

func hasEntryDisabled(entries map[string]EntryConfig, skillNameLower string) bool {
	if len(entries) == 0 {
		return false
	}
	entry, ok := entries[skillNameLower]
	if !ok {
		for k, v := range entries {
			if strings.EqualFold(k, skillNameLower) {
				entry = v
				ok = true
				break
			}
		}
	}
	if !ok || entry.Enabled == nil {
		return false
	}
	return !*entry.Enabled
}

func skillNames(skills []Skill) string {
	names := make([]string, 0, len(skills))
	for _, sk := range skills {
		names = append(names, sk.Name)
	}
	return fmt.Sprintf("%v", names)
}
