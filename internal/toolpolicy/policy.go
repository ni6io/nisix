package toolpolicy

import (
	"strings"
)

type Policy struct {
	Allow []string
	Deny  []string
}

func (p Policy) Allowed(name string) bool {
	if matchesPolicyEntry(p.Deny, name) {
		return false
	}
	if len(p.Allow) == 0 {
		return true
	}
	return matchesPolicyEntry(p.Allow, name)
}

func matchesPolicyEntry(entries []string, name string) bool {
	for _, entry := range entries {
		if matchesPattern(entry, name) {
			return true
		}
	}
	return false
}

func matchesPattern(pattern, name string) bool {
	pattern = strings.TrimSpace(pattern)
	name = strings.TrimSpace(name)
	if pattern == "" || name == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	}
	return pattern == name
}
