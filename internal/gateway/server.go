package gateway

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/ni6io/nisix/internal/bootstrap"
	"github.com/ni6io/nisix/internal/domain"
	"github.com/ni6io/nisix/internal/profile"
	"github.com/ni6io/nisix/internal/security"
	"github.com/ni6io/nisix/internal/sessions"
	"github.com/ni6io/nisix/internal/skills"
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

type Server struct {
	router       Router
	run          Runtime
	hub          ChannelHub
	auth         Authenticator
	sessions     *sessions.Manager
	profileSvc   *profile.Service
	bootstrapSvc *bootstrap.Service
	skillSvc     *skills.Service
	workspace    string
	log          *slog.Logger
}

func New(
	router Router,
	runtime Runtime,
	hub ChannelHub,
	auth Authenticator,
	sessionManager *sessions.Manager,
	profileService *profile.Service,
	bootstrapService *bootstrap.Service,
	skillService *skills.Service,
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
		workspace:    workspaceDir,
		log:          logger,
	}
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
	if s.sessions != nil {
		e, err := s.sessions.Touch(route.SessionKey, route.AgentID)
		if err != nil {
			return err
		}
		sessionEntry = e
		if err := s.sessions.Append(e, "user", msg.Text); err != nil {
			return err
		}
	}

	events := s.run.Run(ctx, domain.RunRequest{
		AgentID:    route.AgentID,
		SessionKey: route.SessionKey,
		RunID:      msg.RunID,
		Message:    msg,
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
		if s.sessions != nil && evt.Done {
			if err := s.sessions.Append(sessionEntry, "assistant", evt.Text); err != nil {
				return err
			}
		}
	}
	return nil
}
