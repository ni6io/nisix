package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ni6io/nisix/internal/domain"
	"github.com/ni6io/nisix/internal/profile"
	"github.com/ni6io/nisix/internal/protocol"
	"github.com/ni6io/nisix/internal/sessions"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

type wsActiveRun struct {
	runID      string
	sessionKey string
	cancel     context.CancelFunc
}

func (s *Server) WSHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		var writeMu sync.Mutex
		var seq uint64
		var runCounter uint64
		var runsMu sync.Mutex
		activeRuns := make(map[string]wsActiveRun)
		connected := false
		sessionToken := ""
		tickStarted := false

		nextSeq := func() uint64 {
			return atomic.AddUint64(&seq, 1)
		}
		nextRunID := func() string {
			n := atomic.AddUint64(&runCounter, 1)
			return fmt.Sprintf("run-%d-%d", time.Now().UnixNano(), n)
		}

		registerRun := func(run wsActiveRun) {
			runsMu.Lock()
			defer runsMu.Unlock()
			activeRuns[run.runID] = run
		}
		unregisterRun := func(runID string) {
			runsMu.Lock()
			defer runsMu.Unlock()
			delete(activeRuns, runID)
		}
		findRunToAbort := func(params protocol.ChatAbortParams) (wsActiveRun, bool) {
			runsMu.Lock()
			defer runsMu.Unlock()
			rid := strings.TrimSpace(params.RunID)
			if rid != "" {
				run, ok := activeRuns[rid]
				return run, ok
			}
			sessionKey := strings.TrimSpace(params.SessionKey)
			if sessionKey == "" {
				return wsActiveRun{}, false
			}
			for _, run := range activeRuns {
				if run.sessionKey == sessionKey {
					return run, true
				}
			}
			return wsActiveRun{}, false
		}

		sendRaw := func(v any) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			return conn.WriteJSON(v)
		}
		sendRes := func(id string, ok bool, payload any, perr *protocol.Error) error {
			return sendRaw(protocol.ResponseFrame{
				Type:    "res",
				ID:      id,
				OK:      ok,
				Payload: payload,
				Error:   perr,
			})
		}
		sendErr := func(id, code, message string) error {
			return sendRes(id, false, nil, &protocol.Error{Code: code, Message: message})
		}
		sendEvent := func(event string, payload any) error {
			return sendRaw(protocol.EventFrame{
				Type:    "event",
				Event:   event,
				Payload: payload,
				Seq:     nextSeq(),
			})
		}

		for {
			var req protocol.RequestFrame
			if err := conn.ReadJSON(&req); err != nil {
				return
			}
			if err := protocol.ValidateRequest(req); err != nil {
				_ = sendErr(req.ID, "INVALID_REQUEST", err.Error())
				continue
			}
			if !connected && req.Method != "connect" {
				_ = sendErr(req.ID, "UNAUTHENTICATED", "first method must be connect")
				return
			}

			switch req.Method {
			case "connect":
				var p protocol.ConnectParams
				if err := decodeParams(req.Params, &p); err != nil {
					_ = sendErr(req.ID, "INVALID_REQUEST", "invalid connect params")
					continue
				}
				if p.MinProtocol > protocol.CurrentProtocol || p.MaxProtocol < protocol.CurrentProtocol {
					_ = sendErr(req.ID, "PROTOCOL_MISMATCH", "unsupported protocol range")
					continue
				}
				if _, err := s.auth.Authenticate(p.Auth.Token); err != nil {
					_ = sendErr(req.ID, "UNAUTHENTICATED", err.Error())
					continue
				}
				sessionToken = p.Auth.Token
				connected = true
				if err := sendRes(req.ID, true, protocol.HelloPayload{
					Type:     "hello-ok",
					Protocol: protocol.CurrentProtocol,
					Features: protocol.Features{
						Methods: []string{
							"health",
							"chat.send",
							"chat.abort",
							"chat.history",
							"sessions.list",
							"skills.list",
							"tools.catalog",
							"mcp.status",
							"mcp.tools",
							"profile.get",
							"profile.update",
							"bootstrap.status",
							"bootstrap.complete",
						},
						Events: []string{"tick", "agent"},
					},
				}, nil); err != nil {
					return
				}
				if !tickStarted {
					tickStarted = true
					go func() {
						t := time.NewTicker(15 * time.Second)
						defer t.Stop()
						for {
							select {
							case <-ctx.Done():
								return
							case now := <-t.C:
								if err := sendEvent("tick", map[string]any{"ts": now.UnixMilli()}); err != nil {
									cancel()
									return
								}
							}
						}
					}()
				}

			case "health":
				if err := sendRes(req.ID, true, map[string]any{"ok": true, "ts": time.Now().UnixMilli()}, nil); err != nil {
					return
				}

			case "sessions.list":
				if err := sendRes(req.ID, true, map[string]any{"sessions": s.SessionsList()}, nil); err != nil {
					return
				}

			case "skills.list":
				var p protocol.SkillsListParams
				_ = decodeParams(req.Params, &p)
				skillsList, err := s.SkillsList(p.EnabledOnly)
				if err != nil {
					_ = sendErr(req.ID, "INTERNAL", err.Error())
					continue
				}
				if err := sendRes(req.ID, true, map[string]any{"skills": skillsList}, nil); err != nil {
					return
				}

			case "tools.catalog":
				var p protocol.ToolsCatalogParams
				_ = decodeParams(req.Params, &p)
				if err := sendRes(req.ID, true, map[string]any{"tools": s.ToolsCatalog()}, nil); err != nil {
					return
				}

			case "mcp.status":
				var p protocol.MCPStatusParams
				_ = decodeParams(req.Params, &p)
				if err := sendRes(req.ID, true, map[string]any{"status": s.MCPStatus()}, nil); err != nil {
					return
				}

			case "mcp.tools":
				var p protocol.MCPToolsParams
				_ = decodeParams(req.Params, &p)
				if err := sendRes(req.ID, true, map[string]any{"tools": s.MCPTools()}, nil); err != nil {
					return
				}

			case "profile.get":
				var p protocol.ProfileGetParams
				if err := decodeParams(req.Params, &p); err != nil {
					_ = sendErr(req.ID, "INVALID_REQUEST", "invalid profile.get params")
					continue
				}
				res, err := s.ProfileGet(p.File)
				if err != nil {
					code, msg := mapProfileError(err)
					_ = sendErr(req.ID, code, msg)
					continue
				}
				if err := sendRes(req.ID, true, res, nil); err != nil {
					return
				}

			case "profile.update":
				var p protocol.ProfileUpdateParams
				if err := decodeParams(req.Params, &p); err != nil {
					_ = sendErr(req.ID, "INVALID_REQUEST", "invalid profile.update params")
					continue
				}
				res, err := s.ProfileUpdate(profile.UpdateRequest{
					File:    p.File,
					Content: p.Content,
					Mode:    profile.UpdateMode(strings.ToLower(strings.TrimSpace(p.Mode))),
					Reason:  p.Reason,
				})
				if err != nil {
					code, msg := mapProfileError(err)
					_ = sendErr(req.ID, code, msg)
					continue
				}
				if err := sendRes(req.ID, true, res, nil); err != nil {
					return
				}

			case "bootstrap.status":
				status, err := s.BootstrapStatus()
				if err != nil {
					_ = sendErr(req.ID, "INTERNAL", err.Error())
					continue
				}
				if err := sendRes(req.ID, true, status, nil); err != nil {
					return
				}

			case "bootstrap.complete":
				var p protocol.BootstrapCompleteParams
				_ = decodeParams(req.Params, &p)
				status, err := s.BootstrapComplete(p.RemoveBootstrap)
				if err != nil {
					_ = sendErr(req.ID, "INTERNAL", err.Error())
					continue
				}
				if err := sendRes(req.ID, true, status, nil); err != nil {
					return
				}

			case "chat.send":
				var p protocol.ChatSendParams
				if err := decodeParams(req.Params, &p); err != nil {
					_ = sendErr(req.ID, "INVALID_REQUEST", "invalid chat.send params")
					continue
				}
				runID := nextRunID()
				msg := domain.InboundMessage{
					Channel:   strings.TrimSpace(p.Channel),
					AccountID: strings.TrimSpace(p.AccountID),
					PeerID:    strings.TrimSpace(p.PeerID),
					PeerType:  parsePeerType(p.PeerType),
					UserID:    strings.TrimSpace(p.UserID),
					Text:      strings.TrimSpace(p.Text),
					ThreadID:  strings.TrimSpace(p.ThreadID),
					RunID:     runID,
					At:        time.Now(),
				}
				route := s.router.Resolve(msg)
				runCtx, runCancel := context.WithCancel(ctx)
				registerRun(wsActiveRun{runID: runID, sessionKey: route.SessionKey, cancel: runCancel})

				if err := sendRes(req.ID, true, map[string]any{
					"status":     "accepted",
					"runId":      runID,
					"sessionKey": route.SessionKey,
				}, nil); err != nil {
					runCancel()
					unregisterRun(runID)
					return
				}

				go func(runID string, sessionKey string, msg domain.InboundMessage, runCtx context.Context) {
					defer unregisterRun(runID)
					err := s.HandleInboundWithObserver(runCtx, sessionToken, msg, func(evt domain.AgentEvent) {
						evtRunID := strings.TrimSpace(evt.RunID)
						if evtRunID == "" {
							evtRunID = runID
						}
						_ = sendEvent("agent", map[string]any{
							"kind":       evt.Kind,
							"runId":      evtRunID,
							"sessionKey": evt.SessionKey,
							"text":       evt.Text,
							"provider":   evt.Provider,
							"toolCall":   evt.ToolCall,
							"usage":      evt.Usage,
							"done":       evt.Done,
							"aborted":    evt.Aborted,
						})
					})
					if err != nil {
						aborted := errors.Is(err, context.Canceled) || runCtx.Err() != nil
						text := "run error: " + err.Error()
						if aborted {
							text = "run aborted"
						}
						_ = sendEvent("agent", map[string]any{
							"kind":       "final",
							"runId":      runID,
							"sessionKey": sessionKey,
							"text":       text,
							"done":       true,
							"aborted":    aborted,
						})
					}
				}(runID, route.SessionKey, msg, runCtx)

			case "chat.abort":
				var p protocol.ChatAbortParams
				if err := decodeParams(req.Params, &p); err != nil {
					_ = sendErr(req.ID, "INVALID_REQUEST", "invalid chat.abort params")
					continue
				}
				run, ok := findRunToAbort(p)
				if !ok {
					_ = sendErr(req.ID, "NOT_FOUND", "run not found")
					continue
				}
				run.cancel()
				if err := sendRes(req.ID, true, map[string]any{
					"aborted":    true,
					"runId":      run.runID,
					"sessionKey": run.sessionKey,
				}, nil); err != nil {
					return
				}

			case "chat.history":
				var p protocol.ChatHistoryParams
				if err := decodeParams(req.Params, &p); err != nil {
					_ = sendErr(req.ID, "INVALID_REQUEST", "invalid chat.history params")
					continue
				}
				p.SessionKey = strings.TrimSpace(p.SessionKey)
				if p.SessionKey == "" {
					_ = sendErr(req.ID, "INVALID_REQUEST", "sessionKey is required")
					continue
				}
				if p.Limit <= 0 {
					p.Limit = 50
				}
				if p.Limit > 500 {
					p.Limit = 500
				}
				var from time.Time
				var to time.Time
				var before time.Time
				var after time.Time
				if p.From > 0 {
					from = time.UnixMilli(p.From).UTC()
				}
				if p.To > 0 {
					to = time.UnixMilli(p.To).UTC()
				}
				if p.Before > 0 {
					before = time.UnixMilli(p.Before).UTC()
				}
				if p.After > 0 {
					after = time.UnixMilli(p.After).UTC()
				}
				history, err := s.ChatHistory(
					sessions.HistoryFilter{
						Limit:  p.Limit,
						Role:   p.Role,
						From:   from,
						To:     to,
						Before: before,
						After:  after,
						Cursor: p.Cursor,
					},
					p.SessionKey,
				)
				if err != nil {
					_ = sendErr(req.ID, "NOT_FOUND", err.Error())
					continue
				}
				if err := sendRes(req.ID, true, map[string]any{
					"messages":   history.Messages,
					"nextCursor": history.NextCursor,
					"prevCursor": history.PrevCursor,
					"total":      history.Total,
				}, nil); err != nil {
					return
				}

			default:
				_ = sendErr(req.ID, "METHOD_NOT_FOUND", "unknown method")
			}
		}
	})
}

func decodeParams(raw any, out any) error {
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func parsePeerType(v string) domain.ChatType {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "group":
		return domain.ChatTypeGroup
	case "channel":
		return domain.ChatTypeChannel
	default:
		return domain.ChatTypeDirect
	}
}

func mapProfileError(err error) (string, string) {
	if err == nil {
		return "INTERNAL", "internal error"
	}
	switch strings.TrimSpace(err.Error()) {
	case "FORBIDDEN_FILE":
		return "FORBIDDEN_FILE", "file is not in profile.allowedFiles"
	case "FILE_TOO_LARGE":
		return "FILE_TOO_LARGE", "content exceeds profile.maxFileBytes"
	case "PROPOSAL_INVALID":
		return "PROPOSAL_INVALID", "proposal is invalid or expired"
	default:
		return "INTERNAL", err.Error()
	}
}
