package profile

import (
	"strings"
)

func ParseCommand(text string) (Command, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Command{}, false
	}
	lower := strings.ToLower(trimmed)
	if lower == "/onboard status" {
		return Command{Kind: CommandOnboardStatus}, true
	}
	if lower == "/onboard done" {
		return Command{Kind: CommandOnboardDone}, true
	}
	if strings.HasPrefix(lower, "/profile apply ") {
		id := strings.TrimSpace(trimmed[len("/profile apply "):])
		if id == "" {
			return Command{}, false
		}
		return Command{Kind: CommandProfileApply, ID: id}, true
	}
	if lower == "/profile list" {
		return Command{Kind: CommandProfileList}, true
	}

	parts := strings.Fields(trimmed)
	if len(parts) < 3 || strings.ToLower(parts[0]) != "/profile" {
		return Command{}, false
	}
	action := strings.ToLower(parts[1])
	file := normalizeFile(parts[2])

	switch action {
	case "show":
		return Command{Kind: CommandProfileShow, File: file}, true
	case "diff":
		body := parseCommandBody(trimmed, parts)
		return Command{Kind: CommandProfileDiff, File: file, Content: body}, true
	case "set", "append":
		body := parseCommandBody(trimmed, parts)
		if action == "set" {
			return Command{Kind: CommandProfileSet, File: file, Content: body}, true
		}
		return Command{Kind: CommandProfileAppend, File: file, Content: body}, true
	}
	return Command{}, false
}

func parseCommandBody(raw string, parts []string) string {
	body := ""
	if len(parts) > 3 {
		body = strings.TrimSpace(raw[strings.Index(raw, parts[3]):])
	}
	if idx := strings.Index(raw, "\n"); idx >= 0 {
		header := strings.TrimSpace(raw[:idx])
		headerParts := strings.Fields(header)
		if len(headerParts) >= 3 {
			body = strings.TrimSpace(raw[idx+1:])
		}
	}
	return body
}
