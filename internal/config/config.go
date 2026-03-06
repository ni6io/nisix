package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Gateway   GatewayConfig   `json:"gateway"`
	Agents    AgentsConfig    `json:"agents"`
	Bindings  []BindingRule   `json:"bindings"`
	Session   SessionConfig   `json:"session"`
	Tools     ToolsConfig     `json:"tools"`
	MCP       MCPConfig       `json:"mcp"`
	Memory    MemoryConfig    `json:"memory"`
	Skills    SkillsConfig    `json:"skills"`
	Model     ModelConfig     `json:"model"`
	Workspace WorkspaceConfig `json:"workspace"`
	Bootstrap BootstrapConfig `json:"bootstrap"`
	Profile   ProfileConfig   `json:"profile"`
	Channels  ChannelsConfig  `json:"channels"`
}

type GatewayConfig struct {
	Bind  string `json:"bind"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

type AgentsConfig struct {
	DefaultID string        `json:"defaultId"`
	List      []AgentConfig `json:"list"`
}

type AgentConfig struct {
	ID        string `json:"id"`
	Workspace string `json:"workspace"`
}

type BindingRule struct {
	AgentID string       `json:"agentId"`
	Match   BindingMatch `json:"match"`
}

type BindingMatch struct {
	Channel   string `json:"channel"`
	AccountID string `json:"accountId"`
	PeerID    string `json:"peerId"`
}

type SessionConfig struct {
	DMMode                  string `json:"dmMode"`
	StateDir                string `json:"stateDir"`
	ContextHistoryLimit     int    `json:"contextHistoryLimit"`
	ContextSummaryMaxChars  int    `json:"contextSummaryMaxChars"`
	ContextSummaryLineChars int    `json:"contextSummaryLineChars"`
}

type ToolsConfig struct {
	Profile string   `json:"profile"`
	Allow   []string `json:"allow"`
	Deny    []string `json:"deny"`
}

type MCPConfig struct {
	Enabled    *bool  `json:"enabled"`
	ConfigFile string `json:"configFile"`
	ToolPrefix string `json:"toolPrefix"`
}

type MemoryConfig struct {
	Enabled       bool   `json:"enabled"`
	AutoLoadScope string `json:"autoLoadScope"`
}

type SkillsConfig struct {
	Enabled      *bool                       `json:"enabled"`
	AutoMatch    *bool                       `json:"autoMatch"`
	MaxInjected  int                         `json:"maxInjected"`
	Allowlist    []string                    `json:"allowlist"`
	Entries      map[string]SkillEntryConfig `json:"entries"`
	MaxBodyChars int                         `json:"maxBodyChars"`
}

type SkillEntryConfig struct {
	Enabled *bool `json:"enabled"`
}

type ModelConfig struct {
	Provider   string      `json:"provider"`
	TimeoutSec int         `json:"timeoutSec"`
	OpenAI     OpenAIModel `json:"openai"`
	Ollama     OllamaModel `json:"ollama"`
}

type OpenAIModel struct {
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
}

type OllamaModel struct {
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
}

type WorkspaceConfig struct {
	BootstrapFromTemplates *bool  `json:"bootstrapFromTemplates"`
	TemplateDir            string `json:"templateDir"`
}

type BootstrapConfig struct {
	ReloadMode string `json:"reloadMode"`
}

type ProfileConfig struct {
	UpdateMode        string   `json:"updateMode"`
	AutoDetectEnabled *bool    `json:"autoDetectEnabled"`
	AllowedFiles      []string `json:"allowedFiles"`
	MaxFileBytes      int      `json:"maxFileBytes"`
}

type ChannelsConfig struct {
	Telegram         TelegramConfig   `json:"telegram"`
	TelegramAccounts []TelegramConfig `json:"telegramAccounts"`
}

type TelegramConfig struct {
	AccountID              string   `json:"accountId"`
	Enabled                bool     `json:"enabled"`
	Token                  string   `json:"token"`
	Polling                bool     `json:"polling"`
	BotUsername            string   `json:"botUsername"`
	AutoDetectBotUsername  *bool    `json:"autoDetectBotUsername"`
	RequireMentionInGroups *bool    `json:"requireMentionInGroups"`
	EnableHelpCommands     *bool    `json:"enableHelpCommands"`
	MinUserIntervalMs      int      `json:"minUserIntervalMs"`
	DedupeWindow           int      `json:"dedupeWindow"`
	AllowlistMode          string   `json:"allowlistMode"`
	AllowUsers             []string `json:"allowUsers"`
	AllowChats             []string `json:"allowChats"`
}

func (c TelegramConfig) RequireMentionInGroupsValue() bool {
	if c.RequireMentionInGroups == nil {
		return true
	}
	return *c.RequireMentionInGroups
}

func (c TelegramConfig) EnableHelpCommandsValue() bool {
	if c.EnableHelpCommands == nil {
		return true
	}
	return *c.EnableHelpCommands
}

func (c TelegramConfig) AutoDetectBotUsernameValue() bool {
	if c.AutoDetectBotUsername == nil {
		return true
	}
	return *c.AutoDetectBotUsername
}

func (c TelegramConfig) AccountIDValue() string {
	return normalizeAccountID(c.AccountID)
}

func (c WorkspaceConfig) BootstrapFromTemplatesValue() bool {
	if c.BootstrapFromTemplates == nil {
		return true
	}
	return *c.BootstrapFromTemplates
}

func (c ProfileConfig) AutoDetectEnabledValue() bool {
	if c.AutoDetectEnabled == nil {
		return true
	}
	return *c.AutoDetectEnabled
}

func (c SkillsConfig) EnabledValue() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

func (c SkillsConfig) AutoMatchValue() bool {
	if c.AutoMatch == nil {
		return true
	}
	return *c.AutoMatch
}

func (c MCPConfig) EnabledValue() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

func Load(path string) (Config, error) {
	var cfg Config
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Gateway.Bind == "" {
		cfg.Gateway.Bind = "127.0.0.1"
	}
	if cfg.Gateway.Port == 0 {
		cfg.Gateway.Port = 18789
	}
	if cfg.Agents.DefaultID == "" {
		cfg.Agents.DefaultID = "main"
	}
	if cfg.Session.StateDir == "" {
		cfg.Session.StateDir = "./state"
	}
	if cfg.Session.ContextHistoryLimit <= 0 {
		cfg.Session.ContextHistoryLimit = 24
	}
	if cfg.Session.ContextSummaryMaxChars <= 0 {
		cfg.Session.ContextSummaryMaxChars = 1800
	}
	if cfg.Session.ContextSummaryLineChars <= 0 {
		cfg.Session.ContextSummaryLineChars = 180
	}
	if cfg.MCP.ConfigFile == "" {
		cfg.MCP.ConfigFile = "./mcp.json"
	}
	if cfg.MCP.ToolPrefix == "" {
		cfg.MCP.ToolPrefix = "mcp"
	}
	if cfg.Skills.MaxInjected <= 0 {
		cfg.Skills.MaxInjected = 1
	}
	if cfg.Skills.MaxBodyChars <= 0 {
		cfg.Skills.MaxBodyChars = 4000
	}
	if cfg.Memory.AutoLoadScope == "" {
		cfg.Memory.AutoLoadScope = "dm_only"
	}
	switch cfg.Memory.AutoLoadScope {
	case "dm_only", "all":
	default:
		cfg.Memory.AutoLoadScope = "dm_only"
	}
	if cfg.Model.Provider == "" {
		cfg.Model.Provider = "echo"
	}
	if cfg.Model.TimeoutSec <= 0 {
		cfg.Model.TimeoutSec = 60
	}
	if cfg.Model.OpenAI.BaseURL == "" {
		cfg.Model.OpenAI.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model.OpenAI.Model == "" {
		cfg.Model.OpenAI.Model = "gpt-5-codex"
	}
	if cfg.Model.OpenAI.APIKey == "" {
		cfg.Model.OpenAI.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	if cfg.Model.Ollama.BaseURL == "" {
		cfg.Model.Ollama.BaseURL = "http://127.0.0.1:11434"
	}
	if cfg.Model.Ollama.Model == "" {
		cfg.Model.Ollama.Model = "llama3.2"
	}
	if cfg.Workspace.TemplateDir == "" {
		cfg.Workspace.TemplateDir = "./workspace/templates"
	}
	if cfg.Bootstrap.ReloadMode == "" {
		cfg.Bootstrap.ReloadMode = "per_message"
	}
	switch cfg.Bootstrap.ReloadMode {
	case "per_message", "session_snapshot":
	default:
		cfg.Bootstrap.ReloadMode = "per_message"
	}
	if cfg.Profile.UpdateMode == "" {
		cfg.Profile.UpdateMode = "hybrid"
	}
	switch cfg.Profile.UpdateMode {
	case "explicit", "hybrid", "auto":
	default:
		cfg.Profile.UpdateMode = "hybrid"
	}
	if cfg.Profile.MaxFileBytes <= 0 {
		cfg.Profile.MaxFileBytes = 262144
	}
	if len(cfg.Profile.AllowedFiles) == 0 {
		cfg.Profile.AllowedFiles = []string{
			"IDENTITY.md",
			"SOUL.md",
			"USER.md",
			"TOOLS.md",
			"AGENTS.md",
			"MEMORY.md",
		}
	}

	applyTelegramDefaults(&cfg.Channels.Telegram)
	for i := range cfg.Channels.TelegramAccounts {
		applyTelegramDefaults(&cfg.Channels.TelegramAccounts[i])
	}
	if err := validateTelegramAccounts(cfg.Channels); err != nil {
		return cfg, err
	}

	if len(cfg.Agents.List) == 0 {
		return cfg, errors.New("config: agents.list is required")
	}
	return cfg, nil
}

func applyTelegramDefaults(cfg *TelegramConfig) {
	cfg.AccountID = normalizeAccountID(cfg.AccountID)
	switch cfg.AllowlistMode {
	case "", "off", "users", "chats", "users_or_chats", "users_and_chats":
		if cfg.AllowlistMode == "" {
			cfg.AllowlistMode = "off"
		}
	default:
		cfg.AllowlistMode = "off"
	}
	if cfg.MinUserIntervalMs < 0 {
		cfg.MinUserIntervalMs = 0
	}
	if cfg.MinUserIntervalMs == 0 {
		cfg.MinUserIntervalMs = 700
	}
	if cfg.DedupeWindow <= 0 {
		cfg.DedupeWindow = 2048
	}
	if cfg.AllowlistMode == "off" {
		if len(cfg.AllowUsers) > 0 && len(cfg.AllowChats) > 0 {
			cfg.AllowlistMode = "users_or_chats"
		} else if len(cfg.AllowUsers) > 0 {
			cfg.AllowlistMode = "users"
		} else if len(cfg.AllowChats) > 0 {
			cfg.AllowlistMode = "chats"
		}
	}
}

func validateTelegramAccounts(ch ChannelsConfig) error {
	seen := map[string]struct{}{}
	register := func(accountID string) error {
		if _, exists := seen[accountID]; exists {
			return fmt.Errorf("config: duplicate telegram accountId %q", accountID)
		}
		seen[accountID] = struct{}{}
		return nil
	}

	if ch.Telegram.Enabled {
		if strings.TrimSpace(ch.Telegram.Token) == "" {
			return errors.New("config: channels.telegram.token is required when enabled")
		}
		if err := register(ch.Telegram.AccountIDValue()); err != nil {
			return err
		}
	}

	for i, account := range ch.TelegramAccounts {
		if !account.Enabled {
			continue
		}
		if strings.TrimSpace(account.Token) == "" {
			return fmt.Errorf("config: channels.telegramAccounts[%d].token is required when enabled", i)
		}
		if err := register(account.AccountIDValue()); err != nil {
			return err
		}
	}

	return nil
}

func normalizeAccountID(v string) string {
	value := strings.ToLower(strings.TrimSpace(v))
	if value == "" {
		return "default"
	}
	return value
}
