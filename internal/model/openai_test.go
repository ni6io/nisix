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

func TestOpenAIClientGenerateUsesResponsesAPI(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %s", got)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","output_text":"hello from codex"}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-5-codex",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	out, err := client.Generate(context.Background(), Request{
		UserText:            "Implement this feature",
		Identity:            domain.AgentIdentity{Name: "Nisix"},
		SoulText:            "Be concise and rigorous.",
		SkillPrompt:         "## Skill: architecture\nUse phased rollout.",
		ConversationSummary: "Earlier the user explained the feature goals.",
		MemoryHits:          []string{"/tmp/memory/foo.md"},
		History: []domain.ConversationMessage{
			{Role: "user", Text: "Remember my name is Thanh."},
			{Role: "assistant", Text: "Understood."},
		},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if out != "hello from codex" {
		t.Fatalf("unexpected output: %q", out)
	}

	if captured["model"] != "gpt-5-codex" {
		t.Fatalf("unexpected model: %#v", captured["model"])
	}
	instructions, _ := captured["instructions"].(string)
	if !strings.Contains(instructions, "SOUL instructions:") || !strings.Contains(instructions, "Active skills:") {
		t.Fatalf("instructions missing expected sections: %q", instructions)
	}
	if !strings.Contains(instructions, "Conversation summary:") {
		t.Fatalf("expected conversation summary in instructions: %q", instructions)
	}
	input := captured["input"].([]any)
	if len(input) != 3 {
		t.Fatalf("expected history + user input, got %#v", input)
	}
	historyUser := input[0].(map[string]any)
	if historyUser["role"] != "user" {
		t.Fatalf("expected history user role, got %#v", historyUser)
	}
	historyText := historyUser["content"].(string)
	if historyText != "Remember my name is Thanh." {
		t.Fatalf("unexpected history text: %q", historyText)
	}
	historyAssistant := input[1].(map[string]any)
	if historyAssistant["role"] != "assistant" {
		t.Fatalf("expected assistant history role, got %#v", historyAssistant)
	}
	if historyAssistant["content"] != "Understood." {
		t.Fatalf("unexpected assistant history text: %#v", historyAssistant["content"])
	}
}

func TestOpenAIClientGenerateParsesOutputArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"content":[{"type":"output_text","text":"from-output-array"}]}]}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-5-codex",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	out, err := client.Generate(context.Background(), Request{UserText: "hi"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if out != "from-output-array" {
		t.Fatalf("unexpected output: %q", out)
	}
}
