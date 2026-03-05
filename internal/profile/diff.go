package profile

import "strings"

func RenderLineDiff(current string, proposed string) string {
	left := splitLines(current)
	right := splitLines(proposed)
	if strings.TrimSpace(current) == strings.TrimSpace(proposed) {
		return "no changes"
	}
	var b strings.Builder
	b.WriteString("--- current\n")
	b.WriteString("+++ proposed\n")
	max := len(left)
	if len(right) > max {
		max = len(right)
	}
	for i := 0; i < max; i++ {
		var l string
		var r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		if l == r {
			continue
		}
		if l != "" {
			b.WriteString("- ")
			b.WriteString(l)
			b.WriteString("\n")
		}
		if r != "" {
			b.WriteString("+ ")
			b.WriteString(r)
			b.WriteString("\n")
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "no changes"
	}
	return out
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
