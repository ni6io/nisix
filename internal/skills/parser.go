package skills

import (
	"errors"
	"strings"
)

func parseSkillMarkdown(content string) (SkillMeta, string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return SkillMeta{}, "", errors.New("empty skill file")
	}
	if !strings.HasPrefix(trimmed, "---") {
		return SkillMeta{}, "", errors.New("missing frontmatter")
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return SkillMeta{}, "", errors.New("invalid frontmatter start")
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return SkillMeta{}, "", errors.New("missing frontmatter end")
	}

	meta := SkillMeta{}
	for _, line := range lines[1:end] {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		v = strings.Trim(v, "\"'")
		switch k {
		case "name":
			meta.Name = v
		case "description":
			meta.Description = v
		}
	}
	if strings.TrimSpace(meta.Name) == "" || strings.TrimSpace(meta.Description) == "" {
		return SkillMeta{}, "", errors.New("frontmatter requires name and description")
	}

	body := strings.Join(lines[end+1:], "\n")
	return meta, strings.TrimSpace(body), nil
}
