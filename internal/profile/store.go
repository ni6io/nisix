package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type fileLocker struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newFileLocker() *fileLocker {
	return &fileLocker{locks: make(map[string]*sync.Mutex)}
}

func (l *fileLocker) lock(path string) func() {
	l.mu.Lock()
	mtx, ok := l.locks[path]
	if !ok {
		mtx = &sync.Mutex{}
		l.locks[path] = mtx
	}
	l.mu.Unlock()
	mtx.Lock()
	return func() { mtx.Unlock() }
}

func applyUpdate(path string, req UpdateRequest) (UpdateResult, error) {
	next, err := previewUpdate(path, req)
	if err != nil {
		return UpdateResult{}, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(next), 0o644); err != nil {
		return UpdateResult{}, fmt.Errorf("INTERNAL")
	}
	if err := os.Rename(tmp, path); err != nil {
		return UpdateResult{}, fmt.Errorf("INTERNAL")
	}
	info, err := os.Stat(path)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("INTERNAL")
	}
	return UpdateResult{
		OK:        true,
		File:      filepath.Base(path),
		UpdatedAt: info.ModTime().UTC(),
		Bytes:     len([]byte(next)),
	}, nil
}

func previewUpdate(path string, req UpdateRequest) (string, error) {
	existing, _ := os.ReadFile(path)
	current := string(existing)
	var next string
	switch req.Mode {
	case UpdateModeAppend:
		next = appendText(current, req.Content)
	case UpdateModePatch:
		patched, err := patchText(filepath.Base(path), current, req.Content)
		if err != nil {
			return "", err
		}
		next = patched
	default:
		next = req.Content
	}
	return next, nil
}

func appendText(current string, extra string) string {
	cur := strings.TrimRight(current, "\n")
	add := strings.TrimSpace(extra)
	if cur == "" {
		return add + "\n"
	}
	if add == "" {
		return cur + "\n"
	}
	return cur + "\n\n" + add + "\n"
}

func patchText(fileName string, current string, patch string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(fileName)) {
	case "IDENTITY.MD":
		return patchIdentity(current, patch), nil
	case "USER.MD":
		return patchUser(current, patch), nil
	default:
		return "", fmt.Errorf("INTERNAL")
	}
}

func patchIdentity(current string, patch string) string {
	lines := strings.Split(strings.TrimSpace(patch), "\n")
	out := current
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		re := regexp.MustCompile(`(?im)^` + regexp.QuoteMeta(key) + `\s*:\s*.*$`)
		if re.MatchString(out) {
			out = re.ReplaceAllString(out, fmt.Sprintf("%s: %s", key, val))
		} else {
			if strings.TrimSpace(out) == "" {
				out = fmt.Sprintf("%s: %s\n", key, val)
			} else {
				out = strings.TrimRight(out, "\n") + "\n" + fmt.Sprintf("%s: %s", key, val) + "\n"
			}
		}
	}
	return ensureTrailingNewline(out)
}

func patchUser(current string, patch string) string {
	out := current
	lines := strings.Split(strings.TrimSpace(patch), "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		label := fmt.Sprintf("- **%s:** %s", key, val)
		re := regexp.MustCompile(`(?im)^- \*\*` + regexp.QuoteMeta(key) + `:\*\*.*$`)
		if re.MatchString(out) {
			out = re.ReplaceAllString(out, label)
			continue
		}
		if strings.TrimSpace(out) == "" {
			out = "# USER\n\n## Profile\n" + label + "\n"
			continue
		}
		if strings.Contains(strings.ToLower(out), "## profile") {
			out = strings.TrimRight(out, "\n") + "\n" + label + "\n"
		} else {
			out = strings.TrimRight(out, "\n") + "\n\n## Profile\n" + label + "\n"
		}
	}
	return ensureTrailingNewline(out)
}

func ensureTrailingNewline(s string) string {
	if s == "" {
		return "\n"
	}
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func fileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime().UTC()
}
