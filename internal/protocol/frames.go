package protocol

import "errors"

const CurrentProtocol = 1

type RequestFrame struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type ResponseFrame struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Payload any    `json:"payload,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type EventFrame struct {
	Type    string `json:"type"`
	Event   string `json:"event"`
	Payload any    `json:"payload,omitempty"`
	Seq     uint64 `json:"seq,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ConnectParams struct {
	MinProtocol int        `json:"minProtocol"`
	MaxProtocol int        `json:"maxProtocol"`
	Client      ClientInfo `json:"client"`
	Auth        AuthInfo   `json:"auth"`
}

type ClientInfo struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
}

type AuthInfo struct {
	Token string `json:"token"`
}

type HelloPayload struct {
	Type     string   `json:"type"`
	Protocol int      `json:"protocol"`
	Features Features `json:"features"`
}

type Features struct {
	Methods []string `json:"methods"`
	Events  []string `json:"events"`
}

type ChatSendParams struct {
	Channel   string `json:"channel"`
	AccountID string `json:"accountId"`
	PeerID    string `json:"peerId"`
	PeerType  string `json:"peerType"`
	UserID    string `json:"userId"`
	ThreadID  string `json:"threadId"`
	Text      string `json:"text"`
}

type ChatHistoryParams struct {
	SessionKey string `json:"sessionKey"`
	Limit      int    `json:"limit"`
	Role       string `json:"role"`
	From       int64  `json:"from"`
	To         int64  `json:"to"`
	Before     int64  `json:"before"`
	After      int64  `json:"after"`
	Cursor     string `json:"cursor"`
}

type SkillsListParams struct {
	EnabledOnly bool `json:"enabledOnly"`
}

type ToolsCatalogParams struct{}

type ChatAbortParams struct {
	RunID      string `json:"runId"`
	SessionKey string `json:"sessionKey"`
}

type ProfileGetParams struct {
	File string `json:"file"`
}

type ProfileUpdateParams struct {
	File    string `json:"file"`
	Content string `json:"content"`
	Mode    string `json:"mode"`
	Reason  string `json:"reason"`
}

type BootstrapCompleteParams struct {
	RemoveBootstrap bool `json:"removeBootstrap"`
}

func ValidateRequest(f RequestFrame) error {
	if f.Type != "req" {
		return errors.New("protocol: request type must be req")
	}
	if f.ID == "" || f.Method == "" {
		return errors.New("protocol: request id and method are required")
	}
	return nil
}
