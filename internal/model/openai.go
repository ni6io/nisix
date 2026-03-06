package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAIConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	Timeout   time.Duration
	UserAgent string
}

type OpenAIClient struct {
	apiKey    string
	baseURL   string
	model     string
	timeout   time.Duration
	userAgent string
	client    *http.Client
}

func NewOpenAIClient(cfg OpenAIConfig) (*OpenAIClient, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("openai: api key is required")
	}
	modelName := strings.TrimSpace(cfg.Model)
	if modelName == "" {
		return nil, fmt.Errorf("openai: model is required")
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		userAgent = "nisix/0.1"
	}
	return &OpenAIClient{
		apiKey:    apiKey,
		baseURL:   strings.TrimRight(baseURL, "/"),
		model:     modelName,
		timeout:   cfg.Timeout,
		userAgent: userAgent,
		client: &http.Client{
			Timeout: 0,
		},
	}, nil
}

func (c *OpenAIClient) Generate(ctx context.Context, req Request) (string, error) {
	if strings.TrimSpace(req.UserText) == "" {
		return "", fmt.Errorf("openai: user text is empty")
	}

	callCtx := ctx
	var cancel context.CancelFunc
	if c.timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	body := map[string]any{
		"model":        c.model,
		"instructions": BuildSystemPrompt(req),
		"input":        buildOpenAIInput(req),
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	reqHTTP, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	reqHTTP.Header.Set("Authorization", "Bearer "+c.apiKey)
	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(reqHTTP)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai: status %d: %s", resp.StatusCode, truncateBody(string(respBody), 500))
	}

	var parsed openAIResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("openai: parse response: %w", err)
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return "", fmt.Errorf("openai: %s", parsed.Error.Message)
	}
	if v := strings.TrimSpace(parsed.OutputText); v != "" {
		return v, nil
	}
	for _, out := range parsed.Output {
		for _, content := range out.Content {
			if v := strings.TrimSpace(content.Text); v != "" {
				return v, nil
			}
		}
	}
	return "", fmt.Errorf("openai: empty response output")
}

func buildOpenAIInput(req Request) []map[string]any {
	input := make([]map[string]any, 0, len(req.History)+1)
	for _, msg := range req.History {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "assistant" {
			role = "user"
		}
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			continue
		}
		input = append(input, map[string]any{
			"role":    role,
			"content": text,
		})
	}
	input = append(input, map[string]any{
		"role":    "user",
		"content": strings.TrimSpace(req.UserText),
	})
	return input
}

type openAIResponse struct {
	OutputText string `json:"output_text"`
	Output     []struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func truncateBody(v string, max int) string {
	if max <= 0 || len(v) <= max {
		return v
	}
	return v[:max] + "...(truncated)"
}
