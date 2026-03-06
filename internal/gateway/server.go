package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ni6io/nisix/internal/bootstrap"
	"github.com/ni6io/nisix/internal/domain"
	"github.com/ni6io/nisix/internal/mcp"
	"github.com/ni6io/nisix/internal/profile"
	"github.com/ni6io/nisix/internal/security"
	"github.com/ni6io/nisix/internal/sessions"
	"github.com/ni6io/nisix/internal/skills"
	"github.com/ni6io/nisix/internal/tools"
	"github.com/ni6io/nisix/internal/workspace"
)

type Router interface {
	Resolve(msg domain.InboundMessage) domain.Route
}

type Runtime interface {
	Run(ctx context.Context, req domain.RunRequest) <-chan domain.AgentEvent
}

type ChannelHub interface {
	Send(ctx context.Context, msg domain.OutboundMessage) error
}

type Authenticator interface {
	Authenticate(token string) (security.Principal, error)
}

type EventObserver func(event domain.AgentEvent)

type MCPInspector interface {
	Status() mcp.StatusSnapshot
	Tools() []mcp.ToolMapping
}

type Server struct {
	router       Router
	run          Runtime
	hub          ChannelHub
	auth         Authenticator
	sessions     *sessions.Manager
	profileSvc   *profile.Service
	bootstrapSvc *bootstrap.Service
	skillSvc     *skills.Service
	toolsReg     *tools.Registry
	mcp          MCPInspector
	workspace    string
	log          *slog.Logger
}

const (
	modelHistoryFetchLimit = 1000000
	modelHistoryWindow     = 24
	modelSummaryMaxItems   = 12
	modelSummaryMaxChars   = 1800
	modelSummaryLineMaxLen = 180
)

func New(
	router Router,
	runtime Runtime,
	hub ChannelHub,
	auth Authenticator,
	sessionManager *sessions.Manager,
	profileService *profile.Service,
	bootstrapService *bootstrap.Service,
	skillService *skills.Service,
	toolsRegistry *tools.Registry,
	workspaceDir string,
	logger *slog.Logger,
) *Server {
	return &Server{
		router:       router,
		run:          runtime,
		hub:          hub,
		auth:         auth,
		sessions:     sessionManager,
		profileSvc:   profileService,
		bootstrapSvc: bootstrapService,
		skillSvc:     skillService,
		toolsReg:     toolsRegistry,
		workspace:    workspaceDir,
		log:          logger,
	}
}

func (s *Server) SetMCPInspector(inspector MCPInspector) {
	s.mcp = inspector
}

func (s *Server) HandleInbound(ctx context.Context, token string, msg domain.InboundMessage) error {
	return s.handleInbound(ctx, token, msg, nil)
}

func (s *Server) HandleInboundWithObserver(
	ctx context.Context,
	token string,
	msg domain.InboundMessage,
	observer EventObserver,
) error {
	return s.handleInbound(ctx, token, msg, observer)
}

func (s *Server) SessionsList() []sessions.Entry {
	if s.sessions == nil {
		return nil
	}
	return s.sessions.List()
}

func (s *Server) ChatHistory(filter sessions.HistoryFilter, sessionKey string) (sessions.HistoryPage, error) {
	if s.sessions == nil {
		return sessions.HistoryPage{}, errors.New("gateway: sessions are not configured")
	}
	filter.Role = strings.ToLower(strings.TrimSpace(filter.Role))
	return s.sessions.HistoryPageFiltered(sessionKey, filter)
}

func (s *Server) SkillsList(enabledOnly bool) ([]skills.SkillStatus, error) {
	if s.skillSvc == nil {
		return nil, nil
	}
	skillsLoaded, err := s.skillSvc.LoadAll(s.workspace)
	if err != nil {
		return nil, err
	}
	out := make([]skills.SkillStatus, 0, len(skillsLoaded))
	for _, skill := range skillsLoaded {
		if enabledOnly && !skill.Enabled {
			continue
		}
		out = append(out, skills.SkillStatus{
			Name:        skill.Name,
			Description: skill.Description,
			Path:        skill.Path,
			Enabled:     skill.Enabled,
			Reason:      skill.Reason,
		})
	}
	return out, nil
}

func (s *Server) ToolsCatalog() []tools.Metadata {
	if s.toolsReg == nil {
		return []tools.Metadata{}
	}
	return s.toolsReg.Catalog()
}

func (s *Server) MCPStatus() mcp.StatusSnapshot {
	if s.mcp == nil {
		return mcp.StatusSnapshot{Available: false}
	}
	return s.mcp.Status()
}

func (s *Server) MCPTools() []mcp.ToolMapping {
	if s.mcp == nil {
		return []mcp.ToolMapping{}
	}
	return s.mcp.Tools()
}

func (s *Server) ProfileGet(file string) (profile.GetResult, error) {
	if s.profileSvc == nil {
		return profile.GetResult{}, errors.New("gateway: profile service is not configured")
	}
	return s.profileSvc.Get(file)
}

func (s *Server) ProfileUpdate(req profile.UpdateRequest) (profile.UpdateResult, error) {
	if s.profileSvc == nil {
		return profile.UpdateResult{}, errors.New("gateway: profile service is not configured")
	}
	return s.profileSvc.Update(req)
}

func (s *Server) BootstrapStatus() (workspace.Status, error) {
	if s.bootstrapSvc != nil {
		return s.bootstrapSvc.Status()
	}
	return workspace.GetStatus(s.workspace)
}

func (s *Server) BootstrapComplete(removeBootstrap bool) (workspace.Status, error) {
	if s.bootstrapSvc != nil {
		return s.bootstrapSvc.Complete(removeBootstrap)
	}
	return workspace.CompleteOnboarding(s.workspace, removeBootstrap)
}

func (s *Server) handleInbound(
	ctx context.Context,
	token string,
	msg domain.InboundMessage,
	observer EventObserver,
) error {
	if _, err := s.auth.Authenticate(token); err != nil {
		return err
	}
	if msg.Channel == "" || msg.PeerID == "" {
		return errors.New("gateway: channel and peerID are required")
	}
	if msg.At.IsZero() {
		msg.At = time.Now()
	}

	route := s.router.Resolve(msg)
	s.log.Info("route.resolved", "agentID", route.AgentID, "sessionKey", route.SessionKey, "matchedBy", route.MatchedBy)

	var sessionEntry sessions.Entry
	history := []domain.ConversationMessage{}
	conversationSummary := ""
	if s.sessions != nil {
		e, err := s.sessions.Touch(route.SessionKey, route.AgentID)
		if err != nil {
			return err
		}
		sessionEntry = e
		history, conversationSummary, err = s.modelHistory(route.SessionKey)
		if err != nil {
			return err
		}
		if err := s.sessions.AppendWithOptions(e, "user", msg.Text, sessions.AppendOptions{
			EventType: "message",
			RunID:     strings.TrimSpace(msg.RunID),
			Kind:      "input",
			Provider:  "runtime",
			Metadata: map[string]string{
				"channel":   msg.Channel,
				"accountId": msg.AccountID,
				"peerId":    msg.PeerID,
				"peerType":  string(msg.PeerType),
				"userId":    msg.UserID,
				"threadId":  msg.ThreadID,
			},
		}); err != nil {
			return err
		}
	}

	events := s.run.Run(ctx, domain.RunRequest{
		AgentID:             route.AgentID,
		SessionKey:          route.SessionKey,
		RunID:               msg.RunID,
		Message:             msg,
		History:             history,
		ConversationSummary: conversationSummary,
	})
	for evt := range events {
		if observer != nil {
			observer(evt)
		}
		if evt.Err != nil {
			return evt.Err
		}
		if evt.Text == "" {
			continue
		}
		if err := s.hub.Send(ctx, domain.OutboundMessage{
			Channel:    msg.Channel,
			AccountID:  msg.AccountID,
			TargetID:   msg.PeerID,
			ThreadID:   msg.ThreadID,
			SessionKey: route.SessionKey,
			Text:       evt.Text,
		}); err != nil {
			return err
		}
		if s.sessions != nil {
			if err := s.sessions.AppendWithOptions(sessionEntry, "assistant", evt.Text, sessions.AppendOptions{
				EventType: mapAgentEventType(evt.Kind),
				RunID:     firstNonEmpty(strings.TrimSpace(evt.RunID), strings.TrimSpace(msg.RunID)),
				Kind:      evt.Kind,
				Provider:  evt.Provider,
				Aborted:   evt.Aborted,
				ToolCall:  mapToolCall(evt.ToolCall),
				Usage:     mapUsage(evt.Usage),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) modelHistory(sessionKey string) ([]domain.ConversationMessage, string, error) {
	page, err := s.sessions.HistoryPageFiltered(sessionKey, sessions.HistoryFilter{Limit: modelHistoryFetchLimit})
	if err != nil {
		return nil, "", err
	}

	allMessages := make([]domain.ConversationMessage, 0, modelHistoryWindow*2)
	for _, rec := range page.Messages {
		if rec.EventType != "" && rec.EventType != "message" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(rec.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		text := strings.TrimSpace(rec.Text)
		if text == "" {
			continue
		}
		allMessages = append(allMessages, domain.ConversationMessage{
			Role: role,
			Text: text,
		})
	}
	if len(allMessages) <= modelHistoryWindow {
		return allMessages, "", nil
	}

	splitAt := len(allMessages) - modelHistoryWindow
	summary := summarizeConversation(allMessages[:splitAt])
	return allMessages[splitAt:], summary, nil
}

func summarizeConversation(messages []domain.ConversationMessage) string {
	if len(messages) == 0 {
		return ""
	}

	selected := sampleConversationMessages(messages, modelSummaryMaxItems)
	lines := make([]string, 0, len(selected)+2)
	lines = append(lines, fmt.Sprintf("Earlier conversation covered %d messages. Key excerpts:", len(messages)))
	for _, msg := range selected {
		text := compactConversationText(msg.Text, modelSummaryLineMaxLen)
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", summarizeRole(msg.Role), text))
	}
	if len(selected) < len(messages) {
		lines = append(lines, fmt.Sprintf("- %d additional earlier messages omitted from this summary.", len(messages)-len(selected)))
	}
	return truncateSummary(strings.Join(lines, "\n"), modelSummaryMaxChars)
}

func sampleConversationMessages(messages []domain.ConversationMessage, maxItems int) []domain.ConversationMessage {
	if len(messages) == 0 || maxItems <= 0 {
		return nil
	}
	if len(messages) <= maxItems {
		return messages
	}

	out := make([]domain.ConversationMessage, 0, maxItems)
	lastIdx := -1
	for i := 0; i < maxItems; i++ {
		idx := i * len(messages) / maxItems
		if idx <= lastIdx {
			idx = lastIdx + 1
		}
		if idx >= len(messages) {
			idx = len(messages) - 1
		}
		out = append(out, messages[idx])
		lastIdx = idx
	}
	return out
}

func compactConversationText(text string, maxLen int) string {
	compacted := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if maxLen <= 0 || len(compacted) <= maxLen {
		return compacted
	}
	if maxLen <= 3 {
		return compacted[:maxLen]
	}
	return compacted[:maxLen-3] + "..."
}

func summarizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return "Assistant"
	default:
		return "User"
	}
}

func truncateSummary(summary string, maxChars int) string {
	if maxChars <= 0 || len(summary) <= maxChars {
		return summary
	}
	if maxChars <= 3 {
		return summary[:maxChars]
	}
	return summary[:maxChars-3] + "..."
}

func mapAgentEventType(kind string) string {
	switch strings.TrimSpace(kind) {
	case "tool":
		return "tool_call"
	case "block":
		return "message_chunk"
	case "final":
		return "message"
	default:
		return "event"
	}
}

func mapToolCall(v *domain.ToolCall) *sessions.ToolCallRecord {
	if v == nil {
		return nil
	}
	return &sessions.ToolCallRecord{
		Name:   v.Name,
		Input:  v.Input,
		Output: v.Output,
		Error:  v.Error,
		Status: v.Status,
	}
}

func mapUsage(v *domain.Usage) *sessions.UsageRecord {
	if v == nil {
		return nil
	}
	return &sessions.UsageRecord{
		InputTokens:  v.InputTokens,
		OutputTokens: v.OutputTokens,
		TotalTokens:  v.TotalTokens,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
