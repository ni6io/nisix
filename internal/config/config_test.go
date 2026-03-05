package config

import (
	"os"
	"path/filepath"
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
