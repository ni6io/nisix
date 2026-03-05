package domain

import "time"

type ChatType string

const (
	ChatTypeDirect  ChatType = "direct"
	ChatTypeGroup   ChatType = "group"
	ChatTypeChannel ChatType = "channel"
)

type InboundMessage struct {
	Channel   string
	AccountID string
	PeerID    string
	PeerType  ChatType
	UserID    string
	Text      string
	ThreadID  string
	RunID     string
	Meta      map[string]string
	At        time.Time
}

type OutboundMessage struct {
	Channel    string
	AccountID  string
	TargetID   string
	ThreadID   string
	SessionKey string
	Text       string
}

type Route struct {
	AgentID    string
	SessionKey string
	MatchedBy  string
	Channel    string
	AccountID  string
}

type RunRequest struct {
	AgentID    string
	SessionKey string
	RunID      string
	Message    InboundMessage
}

type AgentEvent struct {
	Kind       string
	RunID      string
	SessionKey string
	Text       string
	Provider   string
	ToolCall   *ToolCall
	Usage      *Usage
	Done       bool
	Aborted    bool
	Err        error
}

type AgentIdentity struct {
	Name   string
	Avatar string
	Emoji  string
}

type ToolCall struct {
	Name   string         `json:"name"`
	Input  map[string]any `json:"input,omitempty"`
	Output any            `json:"output,omitempty"`
	Error  string         `json:"error,omitempty"`
	Status string         `json:"status,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	TotalTokens  int `json:"totalTokens,omitempty"`
}
