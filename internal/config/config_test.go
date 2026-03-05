package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAppliesModelAndTelegramDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	content := `{
  "gateway": {"token":"t"},
  "agents": {"list":[{"id":"main","workspace":"./workspace/main"}]}
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Model.Provider != "echo" {
		t.Fatalf("expected model.provider echo, got %q", cfg.Model.Provider)
	}
	if cfg.Model.OpenAI.Model != "gpt-5-codex" {
		t.Fatalf("expected default model, got %q", cfg.Model.OpenAI.Model)
	}
	if cfg.Model.Ollama.BaseURL != "http://127.0.0.1:11434" {
		t.Fatalf("expected ollama baseURL default, got %q", cfg.Model.Ollama.BaseURL)
	}
	if cfg.Model.Ollama.Model != "llama3.2" {
		t.Fatalf("expected ollama model default, got %q", cfg.Model.Ollama.Model)
	}
	if !cfg.Workspace.BootstrapFromTemplatesValue() {
		t.Fatal("expected bootstrapFromTemplates default true")
	}
	if cfg.Workspace.TemplateDir != "./workspace/templates" {
		t.Fatalf("expected template dir default, got %q", cfg.Workspace.TemplateDir)
	}
	if cfg.Bootstrap.ReloadMode != "per_message" {
		t.Fatalf("expected bootstrap reload mode per_message, got %q", cfg.Bootstrap.ReloadMode)
	}
	if cfg.Profile.UpdateMode != "hybrid" {
		t.Fatalf("expected profile update mode hybrid, got %q", cfg.Profile.UpdateMode)
	}
	if !cfg.Profile.AutoDetectEnabledValue() {
		t.Fatal("expected profile auto detect default true")
	}
	if cfg.Profile.MaxFileBytes != 262144 {
		t.Fatalf("expected profile max bytes 262144, got %d", cfg.Profile.MaxFileBytes)
	}
	if cfg.Memory.AutoLoadScope != "dm_only" {
		t.Fatalf("expected memory autoLoadScope dm_only, got %q", cfg.Memory.AutoLoadScope)
	}
	if cfg.MCP.ConfigFile != "./mcp.json" {
		t.Fatalf("expected mcp config file default ./mcp.json, got %q", cfg.MCP.ConfigFile)
	}
	if cfg.MCP.ToolPrefix != "mcp" {
		t.Fatalf("expected mcp toolPrefix default mcp, got %q", cfg.MCP.ToolPrefix)
	}
	if !cfg.MCP.EnabledValue() {
		t.Fatal("expected mcp enabled default true")
	}
	if cfg.Channels.Telegram.AllowlistMode != "off" {
		t.Fatalf("expected telegram allowlist mode off, got %q", cfg.Channels.Telegram.AllowlistMode)
	}
	if cfg.Channels.Telegram.MinUserIntervalMs != 700 {
		t.Fatalf("expected minUserIntervalMs default 700, got %d", cfg.Channels.Telegram.MinUserIntervalMs)
	}
	if cfg.Channels.Telegram.DedupeWindow != 2048 {
		t.Fatalf("expected dedupeWindow default 2048, got %d", cfg.Channels.Telegram.DedupeWindow)
	}
	if !cfg.Channels.Telegram.AutoDetectBotUsernameValue() {
		t.Fatal("expected autoDetectBotUsername default true")
	}
	if !cfg.Channels.Telegram.RequireMentionInGroupsValue() {
		t.Fatal("expected requireMentionInGroups default true")
	}
	if !cfg.Channels.Telegram.EnableHelpCommandsValue() {
		t.Fatal("expected enableHelpCommands default true")
	}
	if cfg.Channels.Telegram.AccountIDValue() != "default" {
		t.Fatalf("expected telegram accountId default, got %q", cfg.Channels.Telegram.AccountIDValue())
	}
}

func TestLoadInfersAllowlistModeFromEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	content := `{
  "gateway": {"token":"t"},
  "agents": {"list":[{"id":"main","workspace":"./workspace/main"}]},
  "channels": {
    "telegram": {
      "allowlistMode": "off",
      "allowUsers": ["1001"]
    }
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Channels.Telegram.AllowlistMode != "users" {
		t.Fatalf("expected inferred allowlistMode users, got %q", cfg.Channels.Telegram.AllowlistMode)
	}
}

func TestLoadAppliesDefaultsToTelegramAccounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	content := `{
  "gateway": {"token":"t"},
  "agents": {"list":[{"id":"main","workspace":"./workspace/main"}]},
  "channels": {
    "telegramAccounts": [
      {
        "enabled": true,
        "token": "bot-token-1",
        "allowUsers": ["1001"]
      },
      {
        "enabled": true,
        "accountId": "Work_Bot",
        "token": "bot-token-2",
        "allowChats": ["2001"]
      }
    ]
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Channels.TelegramAccounts) != 2 {
		t.Fatalf("expected 2 telegram accounts, got %d", len(cfg.Channels.TelegramAccounts))
	}
	if cfg.Channels.TelegramAccounts[0].AccountIDValue() != "default" {
		t.Fatalf("expected first accountId default, got %q", cfg.Channels.TelegramAccounts[0].AccountIDValue())
	}
	if cfg.Channels.TelegramAccounts[0].AllowlistMode != "users" {
		t.Fatalf("expected first allowlist inferred users, got %q", cfg.Channels.TelegramAccounts[0].AllowlistMode)
	}
	if cfg.Channels.TelegramAccounts[0].MinUserIntervalMs != 700 {
		t.Fatalf("expected first min interval default 700, got %d", cfg.Channels.TelegramAccounts[0].MinUserIntervalMs)
	}
	if cfg.Channels.TelegramAccounts[1].AccountIDValue() != "work_bot" {
		t.Fatalf("expected second accountId normalized, got %q", cfg.Channels.TelegramAccounts[1].AccountIDValue())
	}
	if cfg.Channels.TelegramAccounts[1].AllowlistMode != "chats" {
		t.Fatalf("expected second allowlist inferred chats, got %q", cfg.Channels.TelegramAccounts[1].AllowlistMode)
	}
}

func TestLoadRejectsDuplicateTelegramAccountIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	content := `{
  "gateway": {"token":"t"},
  "agents": {"list":[{"id":"main","workspace":"./workspace/main"}]},
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "legacy-token",
      "accountId": "default"
    },
    "telegramAccounts": [
      {
        "enabled": true,
        "token": "bot-token-2",
        "accountId": "DEFAULT"
      }
    ]
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected duplicate accountId error")
	}
	if !strings.Contains(err.Error(), "duplicate telegram accountId") {
		t.Fatalf("unexpected error: %v", err)
	}
}
