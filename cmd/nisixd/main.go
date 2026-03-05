package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ni6io/nisix/internal/agentruntime"
	"github.com/ni6io/nisix/internal/bootstrap"
	"github.com/ni6io/nisix/internal/channels"
	"github.com/ni6io/nisix/internal/config"
	"github.com/ni6io/nisix/internal/domain"
	"github.com/ni6io/nisix/internal/gateway"
	"github.com/ni6io/nisix/internal/identity"
	"github.com/ni6io/nisix/internal/memory"
	"github.com/ni6io/nisix/internal/model"
	"github.com/ni6io/nisix/internal/observability"
	"github.com/ni6io/nisix/internal/profile"
	"github.com/ni6io/nisix/internal/router"
	"github.com/ni6io/nisix/internal/security"
	"github.com/ni6io/nisix/internal/sessions"
	"github.com/ni6io/nisix/internal/skills"
	"github.com/ni6io/nisix/internal/soul"
	"github.com/ni6io/nisix/internal/toolpolicy"
	"github.com/ni6io/nisix/internal/tools"
	"github.com/ni6io/nisix/internal/workspace"
)

type inboundRequest struct {
	Token     string `json:"token"`
	Channel   string `json:"channel"`
	AccountID string `json:"accountId"`
	PeerID    string `json:"peerId"`
	PeerType  string `json:"peerType"`
	UserID    string `json:"userId"`
	Text      string `json:"text"`
	ThreadID  string `json:"threadId"`
}

func main() {
	cfgPath := flag.String("config", "configs/nisix.example.json", "path to config file")
	listen := flag.String("listen", ":18789", "listen address")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	workspaceDir, err := resolveWorkspace(cfg)
	if err != nil {
		log.Fatalf("resolve workspace: %v", err)
	}
	if err := workspace.EnsureLayout(workspaceDir, workspace.Options{
		BootstrapFromTemplates: cfg.Workspace.BootstrapFromTemplatesValue(),
		TemplateDir:            cfg.Workspace.TemplateDir,
	}); err != nil {
		log.Fatalf("ensure workspace layout: %v", err)
	}

	logger := observability.New("nisixd")
	if st, err := workspace.GetStatus(workspaceDir); err == nil {
		logger.Info("workspace.bootstrap.seeded",
			"workspace", workspaceDir,
			"seeded", st.Seeded,
			"onboardingCompleted", st.OnboardingCompleted,
			"bootstrapExists", st.BootstrapExists,
		)
	}

	storePath := filepath.Join(cfg.Session.StateDir, "sessions.json")
	transcriptDir := filepath.Join(cfg.Session.StateDir, "transcripts")
	fileStore, err := sessions.NewFileStore(storePath)
	if err != nil {
		log.Fatalf("create session store: %v", err)
	}
	sessionManager := sessions.NewManager(fileStore, transcriptDir)

	reg := tools.NewRegistry()
	reg.Register(tools.NewNowTool())

	identitySvc := identity.NewService(workspaceDir)
	soulSvc := soul.NewService(workspaceDir)
	memorySvc := memory.NewService(workspaceDir)
	bootstrapSvc := bootstrap.NewService(workspaceDir, logger)
	profileSvc := profile.NewService(workspaceDir, profile.Config{
		UpdateMode:        cfg.Profile.UpdateMode,
		AutoDetectEnabled: cfg.Profile.AutoDetectEnabledValue(),
		AllowedFiles:      cfg.Profile.AllowedFiles,
		MaxFileBytes:      cfg.Profile.MaxFileBytes,
		ProposalTTL:       10 * time.Minute,
	}, logger)
	skillsSvc := skills.NewService(skills.Config{
		Enabled:      cfg.Skills.EnabledValue(),
		AutoMatch:    cfg.Skills.AutoMatchValue(),
		MaxInjected:  cfg.Skills.MaxInjected,
		Allowlist:    cfg.Skills.Allowlist,
		Entries:      mapSkillsEntries(cfg.Skills.Entries),
		MaxBodyChars: cfg.Skills.MaxBodyChars,
	}, logger)
	modelClient, err := buildModelClient(cfg.Model)
	if err != nil {
		log.Fatalf("build model client: %v", err)
	}

	runtime := agentruntime.New(
		reg,
		toolpolicy.Policy{Allow: cfg.Tools.Allow, Deny: cfg.Tools.Deny},
		memorySvc,
		identitySvc.Load(),
		soulSvc.Load(),
		workspaceDir,
		bootstrapSvc,
		profileSvc,
		skillsSvc,
		modelClient,
		cfg.Memory.AutoLoadScope,
		cfg.Profile.UpdateMode,
		cfg.Profile.AutoDetectEnabledValue(),
		logger,
	)

	stdoutHub := channels.NewStdoutHub()
	hub := channels.NewMultiHub(stdoutHub)

	srv := gateway.New(
		router.NewResolver(cfg),
		runtime,
		hub,
		security.NewTokenAuthenticator(cfg.Gateway.Token),
		sessionManager,
		profileSvc,
		bootstrapSvc,
		skillsSvc,
		workspaceDir,
		logger,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if cfg.Channels.Telegram.Enabled {
		telegram := channels.NewTelegramAdapter(cfg.Channels.Telegram.Token, channels.TelegramOptions{
			BotUsername:            cfg.Channels.Telegram.BotUsername,
			AutoDetectBotUsername:  cfg.Channels.Telegram.AutoDetectBotUsernameValue(),
			RequireMentionInGroups: cfg.Channels.Telegram.RequireMentionInGroupsValue(),
			EnableHelpCommands:     cfg.Channels.Telegram.EnableHelpCommandsValue(),
			MinUserIntervalMs:      cfg.Channels.Telegram.MinUserIntervalMs,
			DedupeWindow:           cfg.Channels.Telegram.DedupeWindow,
			AllowlistMode:          cfg.Channels.Telegram.AllowlistMode,
			AllowUsers:             cfg.Channels.Telegram.AllowUsers,
			AllowChats:             cfg.Channels.Telegram.AllowChats,
		})
		hub.Register("telegram", telegram)
		logger.Info("telegram.adapter.enabled")
		if cfg.Channels.Telegram.Polling {
			go func() {
				logger.Info("telegram.polling.start")
				err := telegram.RunPolling(ctx, func(msg domain.InboundMessage) error {
					return srv.HandleInbound(ctx, cfg.Gateway.Token, msg)
				})
				if err != nil && !errors.Is(err, context.Canceled) {
					logger.Error("telegram.polling.error", "err", err)
				}
			}()
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.Handle("/ws", srv.WSHandler())
	mux.HandleFunc("/inbound", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		defer r.Body.Close()

		var req inboundRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}

		msg := domain.InboundMessage{
			Channel:   req.Channel,
			AccountID: req.AccountID,
			PeerID:    req.PeerID,
			PeerType:  parsePeerType(req.PeerType),
			UserID:    req.UserID,
			Text:      req.Text,
			ThreadID:  req.ThreadID,
			At:        time.Now(),
		}
		if err := srv.HandleInbound(ctx, req.Token, msg); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	httpServer := &http.Server{
		Addr:    *listen,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	logger.Info("server.start", "listen", *listen, "stateDir", cfg.Session.StateDir)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func resolveWorkspace(cfg config.Config) (string, error) {
	for _, a := range cfg.Agents.List {
		if strings.EqualFold(a.ID, cfg.Agents.DefaultID) {
			if a.Workspace == "" {
				return "", fmt.Errorf("agent %s workspace is empty", a.ID)
			}
			if err := os.MkdirAll(a.Workspace, 0o755); err != nil {
				return "", err
			}
			return a.Workspace, nil
		}
	}
	return "", fmt.Errorf("default agent %s not found", cfg.Agents.DefaultID)
}

func parsePeerType(v string) domain.ChatType {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "group":
		return domain.ChatTypeGroup
	case "channel":
		return domain.ChatTypeChannel
	default:
		return domain.ChatTypeDirect
	}
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func mapSkillsEntries(entries map[string]config.SkillEntryConfig) map[string]skills.EntryConfig {
	out := make(map[string]skills.EntryConfig, len(entries))
	for name, entry := range entries {
		out[name] = skills.EntryConfig{
			Enabled: entry.Enabled,
		}
	}
	return out
}

func buildModelClient(cfg config.ModelConfig) (model.Client, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "echo":
		return model.NewEchoClient(), nil
	case "openai", "codex":
		return model.NewOpenAIClient(model.OpenAIConfig{
			APIKey:  cfg.OpenAI.APIKey,
			BaseURL: cfg.OpenAI.BaseURL,
			Model:   cfg.OpenAI.Model,
			Timeout: time.Duration(cfg.TimeoutSec) * time.Second,
		})
	case "ollama":
		return model.NewOllamaClient(model.OllamaConfig{
			BaseURL: cfg.Ollama.BaseURL,
			Model:   cfg.Ollama.Model,
			Timeout: time.Duration(cfg.TimeoutSec) * time.Second,
		})
	default:
		return nil, fmt.Errorf("unknown model.provider: %s", cfg.Provider)
	}
}
