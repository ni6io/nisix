package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ni6io/nisix/internal/domain"
)

func TestOllamaClientGenerateUsesGenerateAPI(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"hello from ollama","done":true}`))
	}))
	defer srv.Close()

	client, err := NewOllamaClient(OllamaConfig{
		BaseURL: srv.URL,
		Model:   "llama3.2",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	out, err := client.Generate(context.Background(), Request{
		UserText:    "Implement this feature",
		Identity:    domain.AgentIdentity{Name: "Nisix"},
		SoulText:    "Be concise and rigorous.",
		SkillPrompt: "## Skill: architecture\nUse phased rollout.",
		MemoryHits:  []string{"/tmp/memory/foo.md"},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if out != "hello from ollama" {
		t.Fatalf("unexpected output: %q", out)
	}

	if captured["model"] != "llama3.2" {
		t.Fatalf("unexpected model: %#v", captured["model"])
	}
	if captured["stream"] != false {
		t.Fatalf("expected stream=false, got %#v", captured["stream"])
	}
	systemText, _ := captured["system"].(string)
	if !strings.Contains(systemText, "SOUL instructions:") || !strings.Contains(systemText, "Active skills:") {
		t.Fatalf("system prompt missing expected sections: %q", systemText)
	}
	if captured["prompt"] != "Implement this feature" {
		t.Fatalf("unexpected prompt: %#v", captured["prompt"])
	}
}

func TestOllamaClientGenerateParsesErrorField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer srv.Close()

	client, err := NewOllamaClient(OllamaConfig{
		BaseURL: srv.URL,
		Model:   "unknown",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.Generate(context.Background(), Request{UserText: "hi"})
	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("expected model not found error, got %v", err)
	}
}
