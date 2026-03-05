package profile

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Service struct {
	workspace    string
	updateMode   string
	autoDetect   bool
	allowedFiles map[string]struct{}
	maxFileBytes int
	proposals    *proposalStore
	fileLocks    *fileLocker
	log          *slog.Logger
}

func NewService(workspaceDir string, cfg Config, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	files := cfg.AllowedFiles
	if len(files) == 0 {
		files = defaultAllowedFiles()
	}
	allow := make(map[string]struct{}, len(files))
	for _, file := range files {
		name := normalizeFile(file)
		if name == "" {
			continue
		}
		allow[strings.ToUpper(name)] = struct{}{}
	}
	if cfg.MaxFileBytes <= 0 {
		cfg.MaxFileBytes = 262144
	}
	return &Service{
		workspace:    workspaceDir,
		updateMode:   strings.ToLower(strings.TrimSpace(cfg.UpdateMode)),
		autoDetect:   cfg.AutoDetectEnabled,
		allowedFiles: allow,
		maxFileBytes: cfg.MaxFileBytes,
		proposals:    newProposalStore(cfg.ProposalTTL),
		fileLocks:    newFileLocker(),
		log:          logger,
	}
}

func (s *Service) Get(file string) (GetResult, error) {
	name := normalizeFile(file)
	if err := validateFile(name, s.allowedFiles); err != nil {
		return GetResult{}, err
	}
	path := filepath.Join(s.workspace, name)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GetResult{
				File:      name,
				Content:   "",
				UpdatedAt: time.Time{},
				Writable:  true,
			}, nil
		}
		return GetResult{}, fmt.Errorf("INTERNAL")
	}
	return GetResult{
		File:      name,
		Content:   string(b),
		UpdatedAt: fileModTime(path),
		Writable:  true,
	}, nil
}

func (s *Service) Update(req UpdateRequest) (UpdateResult, error) {
	name := normalizeFile(req.File)
	if err := validateFile(name, s.allowedFiles); err != nil {
		s.log.Info("profile.update.rejected", "file", req.File, "reason", "forbidden_file")
		return UpdateResult{}, err
	}
	req.File = name
	req.Mode = normalizeMode(string(req.Mode), s.updateMode)
	if err := validateSize(req.Content, s.maxFileBytes); err != nil {
		s.log.Info("profile.update.rejected", "file", req.File, "reason", "file_too_large")
		return UpdateResult{}, err
	}
	path := filepath.Join(s.workspace, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return UpdateResult{}, fmt.Errorf("INTERNAL")
	}
	unlock := s.fileLocks.lock(path)
	defer unlock()

	s.log.Info("profile.update.requested", "file", req.File, "mode", req.Mode, "reason", req.Reason)
	res, err := applyUpdate(path, req)
	if err != nil {
		s.log.Info("profile.update.rejected", "file", req.File, "reason", err.Error())
		return UpdateResult{}, err
	}
	s.log.Info("profile.update.applied", "file", req.File, "bytes", res.Bytes, "mode", req.Mode)
	return res, nil
}

func (s *Service) MaybeCreateProposal(sessionKey string, text string) (Proposal, bool, error) {
	mode := strings.ToLower(strings.TrimSpace(s.updateMode))
	if !s.autoDetect || !(mode == "hybrid" || mode == "auto") {
		return Proposal{}, false, nil
	}
	req, summary, ok := detectHighConfidence(text)
	if !ok {
		return Proposal{}, false, nil
	}
	if err := validateFile(req.File, s.allowedFiles); err != nil {
		return Proposal{}, false, nil
	}
	if err := validateSize(req.Content, s.maxFileBytes); err != nil {
		return Proposal{}, false, nil
	}
	p := s.proposals.create(sessionKey, req, summary)
	s.log.Info("profile.proposal.created", "id", p.ID, "sessionKey", sessionKey, "file", req.File)
	return p, true, nil
}

func (s *Service) ApplyProposal(sessionKey, id string) (UpdateResult, error) {
	p, ok := s.proposals.get(strings.TrimSpace(id))
	if !ok {
		return UpdateResult{}, fmt.Errorf("PROPOSAL_INVALID")
	}
	if strings.TrimSpace(p.SessionKey) != strings.TrimSpace(sessionKey) {
		return UpdateResult{}, fmt.Errorf("PROPOSAL_INVALID")
	}
	res, err := s.Update(p.Request)
	if err != nil {
		return UpdateResult{}, err
	}
	s.proposals.delete(p.ID)
	s.log.Info("profile.proposal.applied", "id", p.ID, "sessionKey", sessionKey, "file", p.Request.File)
	return res, nil
}

func (s *Service) AllowedFiles() []string {
	out := make([]string, 0, len(s.allowedFiles))
	for file := range s.allowedFiles {
		out = append(out, displayFileName(file))
	}
	sort.Strings(out)
	return out
}

func (s *Service) Preview(req UpdateRequest) (string, error) {
	name := normalizeFile(req.File)
	if err := validateFile(name, s.allowedFiles); err != nil {
		return "", err
	}
	req.File = name
	req.Mode = normalizeMode(string(req.Mode), s.updateMode)
	if err := validateSize(req.Content, s.maxFileBytes); err != nil {
		return "", err
	}
	path := filepath.Join(s.workspace, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("INTERNAL")
	}
	unlock := s.fileLocks.lock(path)
	defer unlock()
	return previewUpdate(path, req)
}

func (s *Service) LatestProposal(sessionKey, file string) (Proposal, bool) {
	return s.proposals.latest(sessionKey, normalizeFile(file))
}

func displayFileName(name string) string {
	parts := strings.SplitN(strings.TrimSpace(name), ".", 2)
	if len(parts) != 2 {
		return strings.TrimSpace(name)
	}
	return strings.ToUpper(parts[0]) + "." + strings.ToLower(parts[1])
}
