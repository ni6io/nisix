package skills

import (
	"regexp"
	"sort"
	"strings"
)

var tokenRe = regexp.MustCompile(`[a-z0-9]+`)

func ExtractExplicitInvocations(message string) []string {
	lines := strings.Split(message, "\n")
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		cmd := strings.ToLower(strings.TrimSpace(fields[0]))
		if cmd != "/skill" && cmd != "!skill" {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(fields[1]))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

type scoredSkill struct {
	skill Skill
	score int
}

func selectAutoMatchedSkills(message string, enabled []Skill, maxInjected int) []Skill {
	msgTokens := tokenize(message)
	if len(msgTokens) == 0 {
		return nil
	}
	scored := make([]scoredSkill, 0, len(enabled))
	for _, s := range enabled {
		haystack := strings.ToLower(strings.TrimSpace(s.Name + " " + s.Description + " " + s.Body))
		score := 0
		for tok := range msgTokens {
			if tok != "" && strings.Contains(haystack, tok) {
				score++
			}
		}
		if score > 0 {
			scored = append(scored, scoredSkill{skill: s, score: score})
		}
	}
	if len(scored) == 0 {
		return nil
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return strings.ToLower(scored[i].skill.Name) < strings.ToLower(scored[j].skill.Name)
	})
	if maxInjected <= 0 {
		maxInjected = 1
	}
	if maxInjected > len(scored) {
		maxInjected = len(scored)
	}
	out := make([]Skill, 0, maxInjected)
	for i := 0; i < maxInjected; i++ {
		out = append(out, scored[i].skill)
	}
	return out
}

func selectExplicitSkills(explicit []string, enabled []Skill, maxInjected int) []Skill {
	if len(explicit) == 0 {
		return nil
	}
	index := make(map[string]Skill, len(enabled))
	for _, s := range enabled {
		index[strings.ToLower(s.Name)] = s
	}
	out := make([]Skill, 0)
	for _, name := range explicit {
		if s, ok := index[strings.ToLower(strings.TrimSpace(name))]; ok {
			out = append(out, s)
		}
	}
	if maxInjected <= 0 {
		maxInjected = 1
	}
	if len(out) > maxInjected {
		out = out[:maxInjected]
	}
	return out
}

func tokenize(text string) map[string]struct{} {
	parts := tokenRe.FindAllString(strings.ToLower(text), -1)
	out := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		out[p] = struct{}{}
	}
	return out
}
