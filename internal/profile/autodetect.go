package profile

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reMyName   = regexp.MustCompile(`(?i)\bmy name is ([\p{L}\p{N} ._\-']{1,64})`)
	reCallMe   = regexp.MustCompile(`(?i)\bcall me ([\p{L}\p{N} ._\-']{1,64})`)
	reYourName = regexp.MustCompile(`(?i)\byour name is ([\p{L}\p{N} ._\-']{1,64})`)
)

func detectHighConfidence(text string) (UpdateRequest, string, bool) {
	msg := strings.TrimSpace(text)
	if msg == "" {
		return UpdateRequest{}, "", false
	}
	if m := reMyName.FindStringSubmatch(msg); len(m) == 2 {
		name := normalizeDetectedValue(m[1])
		if name != "" {
			return UpdateRequest{
				File:    "USER.md",
				Mode:    UpdateModePatch,
				Content: "Name: " + name,
				Reason:  "auto_detect_my_name",
			}, fmt.Sprintf(`Detected preference: update USER.md Name to "%s"`, name), true
		}
	}
	if m := reCallMe.FindStringSubmatch(msg); len(m) == 2 {
		name := normalizeDetectedValue(m[1])
		if name != "" {
			return UpdateRequest{
				File:    "USER.md",
				Mode:    UpdateModePatch,
				Content: "What to call them: " + name,
				Reason:  "auto_detect_call_me",
			}, fmt.Sprintf(`Detected preference: update USER.md What to call them to "%s"`, name), true
		}
	}
	if m := reYourName.FindStringSubmatch(msg); len(m) == 2 {
		name := normalizeDetectedValue(m[1])
		if name != "" {
			return UpdateRequest{
				File:    "IDENTITY.md",
				Mode:    UpdateModePatch,
				Content: "name: " + name,
				Reason:  "auto_detect_your_name",
			}, fmt.Sprintf(`Detected preference: update IDENTITY.md name to "%s"`, name), true
		}
	}
	return UpdateRequest{}, "", false
}

func normalizeDetectedValue(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"'`)
	s = strings.TrimRight(s, ".,!?;:")
	s = strings.TrimSpace(s)
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}
