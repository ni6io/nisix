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

type OllamaConfig struct {
	BaseURL   string
	Model     string
	Timeout   time.Duration
	UserAgent string
}

type OllamaClient struct {
	baseURL   string
	model     string
	timeout   time.Duration
	userAgent string
	client    *http.Client
}

func NewOllamaClient(cfg OllamaConfig) (*OllamaClient, error) {
	modelName := strings.TrimSpace(cfg.Model)
	if modelName == "" {
		return nil, fmt.Errorf("ollama: model is required")
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		userAgent = "nisix/0.1"
	}
	return &OllamaClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		model:     modelName,
		timeout:   cfg.Timeout,
		userAgent: userAgent,
		client: &http.Client{
			Timeout: 0,
		},
	}, nil
}

func (c *OllamaClient) Generate(ctx context.Context, req Request) (string, error) {
	userText := strings.TrimSpace(req.UserText)
	if userText == "" {
		return "", fmt.Errorf("ollama: user text is empty")
	}

	callCtx := ctx
	var cancel context.CancelFunc
	if c.timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	payloadBody := map[string]any{
		"model":  c.model,
		"prompt": userText,
		"system": BuildSystemPrompt(req),
		"stream": false,
	}
	payload, err := json.Marshal(payloadBody)
	if err != nil {
		return "", err
	}

	reqHTTP, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
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
		return "", fmt.Errorf("ollama: status %d: %s", resp.StatusCode, truncateBody(string(respBody), 500))
	}

	var parsed ollamaGenerateResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("ollama: parse response: %w", err)
	}
	if v := strings.TrimSpace(parsed.Error); v != "" {
		return "", fmt.Errorf("ollama: %s", v)
	}
	if v := strings.TrimSpace(parsed.Response); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("ollama: empty response output")
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
	Error    string `json:"error"`
}
