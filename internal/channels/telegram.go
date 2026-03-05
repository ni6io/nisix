package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ni6io/nisix/internal/domain"
)

type TelegramAdapter struct {
	accountID              string
	token                  string
	client                 *http.Client
	apiBaseURL             string
	botUsername            string
	autoDetectBotUsername  bool
	requireMentionInGroups bool
	enableHelpCommands     bool
	minUserInterval        time.Duration
	dedupeWindow           int
	allowlistMode          string
	allowUsers             map[string]struct{}
	allowChats             map[string]struct{}
	mu                     sync.Mutex
	lastInboundByUser      map[string]time.Time
	seenUpdateOrder        []int
	seenUpdateSet          map[int]struct{}
}

type TelegramOptions struct {
	AccountID              string
	APIBaseURL             string
	BotUsername            string
	AutoDetectBotUsername  bool
	RequireMentionInGroups bool
	EnableHelpCommands     bool
	MinUserIntervalMs      int
	DedupeWindow           int
	AllowlistMode          string
	AllowUsers             []string
	AllowChats             []string
}

func NewTelegramAdapter(token string, options ...TelegramOptions) *TelegramAdapter {
	cfg := TelegramOptions{
		AccountID:              "default",
		APIBaseURL:             "https://api.telegram.org",
		AutoDetectBotUsername:  true,
		RequireMentionInGroups: true,
		EnableHelpCommands:     true,
		MinUserIntervalMs:      700,
		DedupeWindow:           2048,
		AllowlistMode:          "off",
	}
	if len(options) > 0 {
		cfg = options[0]
		if strings.TrimSpace(cfg.AccountID) == "" {
			cfg.AccountID = "default"
		}
		if strings.TrimSpace(cfg.APIBaseURL) == "" {
			cfg.APIBaseURL = "https://api.telegram.org"
		}
		if strings.TrimSpace(cfg.AllowlistMode) == "" {
			cfg.AllowlistMode = "off"
		}
	}
	if cfg.MinUserIntervalMs < 0 {
		cfg.MinUserIntervalMs = 0
	}
	if cfg.DedupeWindow <= 0 {
		cfg.DedupeWindow = 2048
	}

	return &TelegramAdapter{
		accountID: normalizeAccountID(cfg.AccountID),
		token:     strings.TrimSpace(token),
		client: &http.Client{
			Timeout: 40 * time.Second,
		},
		apiBaseURL:             strings.TrimRight(strings.TrimSpace(cfg.APIBaseURL), "/"),
		botUsername:            normalizeUsername(cfg.BotUsername),
		autoDetectBotUsername:  cfg.AutoDetectBotUsername,
		requireMentionInGroups: cfg.RequireMentionInGroups,
		enableHelpCommands:     cfg.EnableHelpCommands,
		minUserInterval:        time.Duration(cfg.MinUserIntervalMs) * time.Millisecond,
		dedupeWindow:           cfg.DedupeWindow,
		allowlistMode:          normalizeAllowlistMode(cfg.AllowlistMode),
		allowUsers:             toSet(cfg.AllowUsers),
		allowChats:             toSet(cfg.AllowChats),
		lastInboundByUser:      make(map[string]time.Time),
		seenUpdateSet:          make(map[int]struct{}),
	}
}

type telegramEnvelope[T any] struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Result      T      `json:"result"`
}

type telegramUpdate struct {
	UpdateID int              `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramMessage struct {
	MessageID       int           `json:"message_id"`
	MessageThreadID int           `json:"message_thread_id"`
	Text            string        `json:"text"`
	Chat            telegramChat  `json:"chat"`
	From            *telegramUser `json:"from"`
}

type telegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type telegramUser struct {
	ID    int64 `json:"id"`
	IsBot bool  `json:"is_bot"`
}

func (t *TelegramAdapter) Send(ctx context.Context, msg domain.OutboundMessage) error {
	chatID, err := strconv.ParseInt(strings.TrimSpace(msg.TargetID), 10, 64)
	if err != nil {
		return fmt.Errorf("telegram: invalid target id %q", msg.TargetID)
	}
	body := map[string]any{
		"chat_id": chatID,
		"text":    msg.Text,
	}
	if threadID := strings.TrimSpace(msg.ThreadID); threadID != "" {
		if parsed, err := strconv.Atoi(threadID); err == nil {
			body["message_thread_id"] = parsed
		}
	}
	_, err = t.call(ctx, "sendMessage", body)
	return err
}

func (t *TelegramAdapter) RunPolling(
	ctx context.Context,
	onInbound func(msg domain.InboundMessage) error,
) error {
	t.ensureBotUsername(ctx)

	offset := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		updates, err := t.getUpdates(ctx, offset)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		for _, upd := range updates {
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}
			if t.isDuplicateUpdate(upd.UpdateID) {
				continue
			}
			if upd.Message == nil {
				continue
			}
			msg := upd.Message
			if msg.From != nil && msg.From.IsBot {
				continue
			}
			if strings.TrimSpace(msg.Text) == "" {
				continue
			}

			threadID := ""
			if msg.MessageThreadID > 0 {
				threadID = strconv.Itoa(msg.MessageThreadID)
			}
			userID := strconv.FormatInt(msg.Chat.ID, 10)
			if msg.From != nil {
				userID = strconv.FormatInt(msg.From.ID, 10)
			}
			if !t.allowByRateLimit(userID, time.Now()) {
				continue
			}
			if !t.allowedByAllowlist(strconv.FormatInt(msg.Chat.ID, 10), userID) {
				continue
			}
			text := strings.TrimSpace(msg.Text)
			if t.enableHelpCommands && t.isHelpOrStartCommand(text) {
				_ = t.Send(ctx, domain.OutboundMessage{
					Channel:  "telegram",
					TargetID: strconv.FormatInt(msg.Chat.ID, 10),
					ThreadID: threadID,
					Text:     t.helpText(),
				})
				continue
			}
			if !t.acceptByMentionPolicy(msg.Chat.Type, text) {
				continue
			}
			text = t.sanitizeText(msg.Chat.Type, text)
			if text == "" {
				continue
			}
			inbound := domain.InboundMessage{
				Channel:   "telegram",
				AccountID: t.accountID,
				PeerID:    strconv.FormatInt(msg.Chat.ID, 10),
				PeerType:  mapTelegramChatType(msg.Chat.Type),
				UserID:    userID,
				Text:      text,
				ThreadID:  threadID,
				At:        time.Now(),
			}
			if err := onInbound(inbound); err != nil {
				return err
			}
		}
	}
}

func (t *TelegramAdapter) getUpdates(ctx context.Context, offset int) ([]telegramUpdate, error) {
	params := map[string]any{
		"offset":          offset,
		"timeout":         25,
		"allowed_updates": []string{"message"},
	}
	b, err := t.call(ctx, "getUpdates", params)
	if err != nil {
		return nil, err
	}
	var env telegramEnvelope[[]telegramUpdate]
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	if !env.OK {
		return nil, fmt.Errorf("telegram getUpdates failed: %s", env.Description)
	}
	return env.Result, nil
}

func (t *TelegramAdapter) call(ctx context.Context, method string, payload map[string]any) ([]byte, error) {
	if t.token == "" {
		return nil, fmt.Errorf("telegram: token is empty")
	}
	endpoint := fmt.Sprintf("%s/bot%s/%s", t.apiBaseURL, url.PathEscape(t.token), method)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram api status %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func mapTelegramChatType(chatType string) domain.ChatType {
	switch strings.ToLower(strings.TrimSpace(chatType)) {
	case "group", "supergroup":
		return domain.ChatTypeGroup
	case "channel":
		return domain.ChatTypeChannel
	default:
		return domain.ChatTypeDirect
	}
}

func (t *TelegramAdapter) ensureBotUsername(ctx context.Context) {
	if !t.autoDetectBotUsername {
		return
	}
	t.mu.Lock()
	hasUsername := t.botUsername != ""
	t.mu.Unlock()
	if hasUsername {
		return
	}
	params := map[string]any{}
	b, err := t.call(ctx, "getMe", params)
	if err != nil {
		return
	}
	var env telegramEnvelope[struct {
		Username string `json:"username"`
	}]
	if err := json.Unmarshal(b, &env); err != nil {
		return
	}
	if !env.OK {
		return
	}
	username := normalizeUsername(env.Result.Username)
	if username == "" {
		return
	}
	t.mu.Lock()
	t.botUsername = username
	t.mu.Unlock()
}

func normalizeAllowlistMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "users", "chats", "users_or_chats", "users_and_chats":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return "off"
	}
}

func normalizeAccountID(v string) string {
	id := strings.ToLower(strings.TrimSpace(v))
	if id == "" {
		return "default"
	}
	return id
}

func normalizeUsername(v string) string {
	u := strings.TrimSpace(strings.ToLower(v))
	return strings.TrimPrefix(u, "@")
}

func toSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		k := strings.TrimSpace(v)
		if k == "" {
			continue
		}
		out[k] = struct{}{}
	}
	return out
}

func (t *TelegramAdapter) allowedByAllowlist(chatID, userID string) bool {
	switch t.allowlistMode {
	case "users":
		_, ok := t.allowUsers[userID]
		return ok
	case "chats":
		_, ok := t.allowChats[chatID]
		return ok
	case "users_or_chats":
		_, userOK := t.allowUsers[userID]
		_, chatOK := t.allowChats[chatID]
		return userOK || chatOK
	case "users_and_chats":
		_, userOK := t.allowUsers[userID]
		_, chatOK := t.allowChats[chatID]
		return userOK && chatOK
	default:
		return true
	}
}

func (t *TelegramAdapter) allowByRateLimit(userID string, now time.Time) bool {
	if t.minUserInterval <= 0 {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	last, ok := t.lastInboundByUser[userID]
	if ok && now.Sub(last) < t.minUserInterval {
		return false
	}
	t.lastInboundByUser[userID] = now
	return true
}

func (t *TelegramAdapter) isDuplicateUpdate(updateID int) bool {
	if updateID <= 0 || t.dedupeWindow <= 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.seenUpdateSet[updateID]; exists {
		return true
	}
	t.seenUpdateSet[updateID] = struct{}{}
	t.seenUpdateOrder = append(t.seenUpdateOrder, updateID)
	if len(t.seenUpdateOrder) > t.dedupeWindow {
		oldest := t.seenUpdateOrder[0]
		t.seenUpdateOrder = t.seenUpdateOrder[1:]
		delete(t.seenUpdateSet, oldest)
	}
	return false
}

func (t *TelegramAdapter) isHelpOrStartCommand(text string) bool {
	first := text
	if parts := strings.Fields(strings.TrimSpace(text)); len(parts) > 0 {
		first = parts[0]
	}
	first = strings.TrimPrefix(strings.TrimSpace(first), "/")
	if first == "" {
		return false
	}
	cmd := strings.ToLower(first)
	if strings.Contains(cmd, "@") {
		parts := strings.SplitN(cmd, "@", 2)
		cmd = parts[0]
		botUsername := t.getBotUsername()
		if botUsername != "" && len(parts) == 2 && parts[1] != botUsername {
			return false
		}
	}
	return cmd == "start" || cmd == "help"
}

func (t *TelegramAdapter) helpText() string {
	if username := t.getBotUsername(); username != "" {
		return fmt.Sprintf("Nisix bot is online.\nUse /help for this message.\nIn groups, mention @%s in your prompt.", username)
	}
	return "Nisix bot is online.\nUse /help for this message."
}

func (t *TelegramAdapter) acceptByMentionPolicy(chatType, text string) bool {
	if !t.requireMentionInGroups {
		return true
	}
	c := strings.ToLower(strings.TrimSpace(chatType))
	if c != "group" && c != "supergroup" {
		return true
	}
	botUsername := t.getBotUsername()
	msg := strings.ToLower(text)
	if strings.HasPrefix(strings.TrimSpace(msg), "/") {
		first := strings.TrimSpace(msg)
		if parts := strings.Fields(first); len(parts) > 0 {
			first = strings.TrimPrefix(parts[0], "/")
			if strings.Contains(first, "@") {
				cmdParts := strings.SplitN(first, "@", 2)
				if botUsername == "" {
					return false
				}
				return len(cmdParts) == 2 && strings.EqualFold(cmdParts[1], botUsername)
			}
		}
		return true
	}
	if botUsername == "" {
		return false
	}
	return strings.Contains(msg, "@"+botUsername)
}

func (t *TelegramAdapter) sanitizeText(chatType, text string) string {
	c := strings.ToLower(strings.TrimSpace(chatType))
	if c != "group" && c != "supergroup" {
		return strings.TrimSpace(text)
	}
	botUsername := t.getBotUsername()
	if botUsername == "" {
		return strings.TrimSpace(text)
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	out := make([]string, 0, len(fields))
	for i, f := range fields {
		lower := strings.ToLower(f)
		if lower == "@"+botUsername {
			continue
		}
		if i == 0 && strings.HasPrefix(f, "/") && strings.Contains(f, "@") {
			parts := strings.SplitN(f, "@", 2)
			if len(parts) == 2 && strings.EqualFold(parts[1], botUsername) {
				f = parts[0]
			}
		}
		out = append(out, f)
	}
	return strings.TrimSpace(strings.Join(out, " "))
}

func (t *TelegramAdapter) getBotUsername() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.botUsername
}
