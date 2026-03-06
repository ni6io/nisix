package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/ni6io/nisix/internal/bootstrap"
	"github.com/ni6io/nisix/internal/domain"
	"github.com/ni6io/nisix/internal/mcp"
	"github.com/ni6io/nisix/internal/memory"
	"github.com/ni6io/nisix/internal/model"
	"github.com/ni6io/nisix/internal/profile"
	"github.com/ni6io/nisix/internal/skills"
	"github.com/ni6io/nisix/internal/toolpolicy"
	"github.com/ni6io/nisix/internal/tools"
)

var toolCallLinePattern = regexp.MustCompile(`^\s*(?:[A-Za-z_][A-Za-z0-9_]*\s*:\s*)?([a-z][a-z0-9_]*)\s*\(\s*(\{.*\})?\s*\)\s*$`)
var inlineCodePattern = regexp.MustCompile("`([^`]+)`")

type Runtime struct {
	tools               *tools.Registry
	policy              toolpolicy.Policy
	mcp                 mcpInspector
	memory              *memory.Service
	identity            domain.AgentIdentity
	soulText            string
	workspaceDir        string
	bootstrap           *bootstrap.Service
	profile             *profile.Service
	skills              *skills.Service
	model               model.Client
	memoryAutoLoadScope string
	profileUpdateMode   string
	profileAutoDetect   bool
	log                 *slog.Logger
}

type mcpInspector interface {
	Status() mcp.StatusSnapshot
	Tools() []mcp.ToolMapping
}

func New(
	registry *tools.Registry,
	policy toolpolicy.Policy,
	memoryService *memory.Service,
	identity domain.AgentIdentity,
	soulText string,
	workspaceDir string,
	bootstrapService *bootstrap.Service,
	profileService *profile.Service,
	skillService *skills.Service,
	modelClient model.Client,
	memoryAutoLoadScope string,
	profileUpdateMode string,
	profileAutoDetect bool,
	logger *slog.Logger,
) *Runtime {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runtime{
		tools:               registry,
		policy:              policy,
		memory:              memoryService,
		identity:            identity,
		soulText:            soulText,
		workspaceDir:        workspaceDir,
		bootstrap:           bootstrapService,
		profile:             profileService,
		skills:              skillService,
		model:               modelClient,
		memoryAutoLoadScope: strings.ToLower(strings.TrimSpace(memoryAutoLoadScope)),
		profileUpdateMode:   strings.ToLower(strings.TrimSpace(profileUpdateMode)),
		profileAutoDetect:   profileAutoDetect,
		log:                 logger,
	}
}

func (r *Runtime) SetMCPInspector(inspector mcpInspector) {
	r.mcp = inspector
}

func (r *Runtime) Run(ctx context.Context, req domain.RunRequest) <-chan domain.AgentEvent {
	out := make(chan domain.AgentEvent, 4)
	go func() {
		defer close(out)

		runID := strings.TrimSpace(req.RunID)
		if runID == "" {
			runID = fmt.Sprintf("run-%d", time.Now().UnixNano())
		}
		text := strings.TrimSpace(req.Message.Text)
		skillContext := ""
		toolContext := r.buildToolPrompt()

		if handled, response := r.handleCommand(req, text); handled {
			out <- domain.AgentEvent{Kind: "final", RunID: runID, SessionKey: req.SessionKey, Text: response, Done: true}
			return
		}

		if r.profile != nil && r.profileAutoDetect && r.profileUpdateMode != "explicit" {
			p, ok, err := r.profile.MaybeCreateProposal(req.SessionKey, text)
			if err != nil {
				r.log.Warn("profile.proposal.error", "err", err)
			}
			if ok {
				out <- domain.AgentEvent{
					Kind:       "final",
					RunID:      runID,
					SessionKey: req.SessionKey,
					Text:       fmt.Sprintf("%s\nProposal ID: %s\nApply with: /profile apply %s", p.Summary, p.ID, p.ID),
					Done:       true,
				}
				return
			}
		}

		if r.skills != nil && r.workspaceDir != "" {
			loaded, err := r.skills.LoadAll(r.workspaceDir)
			if err != nil {
				r.log.Warn("skills.load.error", "err", err)
			}
			explicit := skills.ExtractExplicitInvocations(text)
			selected, err := r.skills.SelectForMessage(text, explicit)
			if err != nil {
				r.log.Warn("skills.select.error", "err", err)
			}

			if len(explicit) > 0 && len(selected) == 0 {
				reasons := make([]string, 0, len(explicit))
				for _, name := range explicit {
					found := false
					for _, sk := range loaded {
						if strings.EqualFold(sk.Name, name) {
							found = true
							if sk.Enabled {
								reasons = append(reasons, fmt.Sprintf("%s unavailable", name))
							} else {
								reasons = append(reasons, fmt.Sprintf("%s blocked (%s)", name, sk.Reason))
							}
							break
						}
					}
					if !found {
						reasons = append(reasons, fmt.Sprintf("%s not found", name))
					}
				}
				out <- domain.AgentEvent{
					Kind:       "final",
					RunID:      runID,
					SessionKey: req.SessionKey,
					Text:       "skill request rejected: " + strings.Join(reasons, "; "),
					Done:       true,
				}
				return
			}

			if len(selected) > 0 {
				parts := make([]string, 0, len(selected))
				for _, sk := range selected {
					parts = append(parts, fmt.Sprintf("## Skill: %s\n%s", sk.Name, sk.Body))
				}
				skillContext = strings.Join(parts, "\n\n")
			}
		}

		if strings.HasPrefix(text, "!slow ") || text == "!slow" {
			parts := strings.SplitN(text, " ", 2)
			payload := "processing"
			if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
				payload = strings.TrimSpace(parts[1])
			}
			for i := 0; i < 20; i++ {
				select {
				case <-ctx.Done():
					out <- domain.AgentEvent{Kind: "final", RunID: runID, SessionKey: req.SessionKey, Text: "run aborted", Done: true, Aborted: true}
					return
				case <-time.After(150 * time.Millisecond):
				}
				out <- domain.AgentEvent{Kind: "block", RunID: runID, SessionKey: req.SessionKey, Text: fmt.Sprintf("chunk %d: %s", i+1, payload)}
			}
			out <- domain.AgentEvent{Kind: "final", RunID: runID, SessionKey: req.SessionKey, Text: "slow run complete", Done: true}
			return
		}

		if strings.HasPrefix(text, "!tool ") {
			parts := strings.Fields(text)
			if len(parts) >= 2 {
				name := parts[1]
				if !r.policy.Allowed(name) {
					out <- domain.AgentEvent{Kind: "final", RunID: runID, SessionKey: req.SessionKey, Text: "tool blocked by policy", Provider: "tool", Done: true}
					return
				}
				res, err := r.tools.Execute(ctx, name, map[string]any{})
				if err != nil {
					out <- domain.AgentEvent{
						Kind:       "final",
						RunID:      runID,
						SessionKey: req.SessionKey,
						Text:       "tool error: " + err.Error(),
						Provider:   "tool",
						ToolCall: &domain.ToolCall{
							Name:   name,
							Input:  map[string]any{},
							Error:  err.Error(),
							Status: "error",
						},
						Done: true,
					}
					return
				}
				out <- domain.AgentEvent{
					Kind:       "tool",
					RunID:      runID,
					SessionKey: req.SessionKey,
					Text:       fmt.Sprintf("tool %s result: %+v", name, res.Data),
					Provider:   "tool",
					ToolCall: &domain.ToolCall{
						Name:   name,
						Input:  map[string]any{},
						Output: res.Data,
						Status: "success",
					},
				}
				out <- domain.AgentEvent{Kind: "final", RunID: runID, SessionKey: req.SessionKey, Text: "done", Done: true}
				return
			}
		}

		select {
		case <-ctx.Done():
			out <- domain.AgentEvent{Kind: "final", RunID: runID, SessionKey: req.SessionKey, Text: "run aborted", Done: true, Aborted: true}
			return
		default:
		}

		identity := r.identity
		soulText := r.soulText
		projectContext := ""
		if r.bootstrap != nil {
			ctxBundle, err := r.bootstrap.LoadContext(req.SessionKey, req.Message)
			if err != nil {
				r.log.Warn("bootstrap.load.error", "err", err)
			} else {
				if strings.TrimSpace(ctxBundle.Identity.Name) != "" {
					identity = ctxBundle.Identity
				}
				if strings.TrimSpace(ctxBundle.SoulText) != "" {
					soulText = ctxBundle.SoulText
				}
				projectContext = ctxBundle.ProjectPrompt
			}
		}

		memHits := []string{}
		scope := r.memoryAutoLoadScope
		if scope == "" {
			scope = "dm_only"
		}
		if r.memory != nil {
			if scope == "dm_only" && req.Message.PeerType != domain.ChatTypeDirect {
				r.log.Info("memory.scope.skipped", "scope", scope, "peerType", req.Message.PeerType, "sessionKey", req.SessionKey)
			} else {
				memHits, _ = r.memory.Search(text)
			}
		}

		if r.model != nil {
			generated, err := r.model.Generate(ctx, model.Request{
				AgentID:             req.AgentID,
				SessionKey:          req.SessionKey,
				UserText:            text,
				History:             req.History,
				ConversationSummary: req.ConversationSummary,
				Identity:            identity,
				SoulText:            soulText,
				ProjectContext:      projectContext,
				SkillPrompt:         skillContext,
				ToolPrompt:          toolContext,
				MemoryHits:          memHits,
			})
			if err != nil {
				out <- domain.AgentEvent{Kind: "final", RunID: runID, SessionKey: req.SessionKey, Text: "model error: " + err.Error(), Provider: "model", Done: true}
				return
			}
			if toolEvent, finalText, toolCall, handled := r.maybeExecuteGeneratedToolCall(ctx, generated); handled {
				if strings.TrimSpace(toolEvent) != "" {
					out <- domain.AgentEvent{Kind: "tool", RunID: runID, SessionKey: req.SessionKey, Text: toolEvent, Provider: "tool", ToolCall: toolCall}
				}
				out <- domain.AgentEvent{Kind: "final", RunID: runID, SessionKey: req.SessionKey, Text: finalText, Provider: "tool", Done: true}
				return
			}
			r.log.Info("runtime.complete", "agentID", req.AgentID, "sessionKey", req.SessionKey, "provider", "model")
			out <- domain.AgentEvent{Kind: "final", RunID: runID, SessionKey: req.SessionKey, Text: generated, Provider: "model", Done: true}
			return
		}

		prefix := identity.Name
		if prefix == "" {
			prefix = "Assistant"
		}
		reply := fmt.Sprintf("%s: %s", prefix, text)
		if len(memHits) > 0 {
			reply += fmt.Sprintf(" (memory hits: %d)", len(memHits))
		}
		if strings.TrimSpace(soulText) != "" {
			reply += " [soul loaded]"
		}
		if strings.TrimSpace(skillContext) != "" {
			reply += "\n\n" + skillContext
		}
		if strings.TrimSpace(toolContext) != "" {
			reply += "\n\n" + toolContext
		}
		if strings.TrimSpace(projectContext) != "" {
			reply += "\n\n" + projectContext
		}
		r.log.Info("runtime.complete", "agentID", req.AgentID, "sessionKey", req.SessionKey)
		out <- domain.AgentEvent{Kind: "final", RunID: runID, SessionKey: req.SessionKey, Text: reply, Done: true}
	}()
	return out
}

func (r *Runtime) buildToolPrompt() string {
	if r.tools == nil {
		return ""
	}
	catalog := r.tools.Catalog()
	if len(catalog) == 0 {
		return ""
	}

	lines := []string{
		"If a tool is required, respond with exactly one tool call and no extra prose.",
		"Valid syntax:",
		"- tool_name()",
		"- tool_name({\"key\":\"value\"})",
		"Never fabricate command output, filesystem listings, timestamps, or external results. Use a tool whenever the answer depends on runtime state.",
		"Only call tools from this allowlisted set:",
	}
	allowedCount := 0
	for _, tool := range catalog {
		if !r.policy.Allowed(tool.Name) {
			continue
		}
		line := fmt.Sprintf("- %s", tool.Name)
		if desc := strings.TrimSpace(tool.Description); desc != "" {
			line += " - " + desc
		}
		if required := describeRequiredFields(tool.InputSchema); required != "" {
			line += " Required: " + required + "."
		}
		if example := buildToolExample(tool.Name, tool.InputSchema); example != "" {
			line += " Example: " + example + "."
		}
		lines = append(lines, line)
		allowedCount++
	}
	if allowedCount == 0 {
		return ""
	}
	lines = append(lines, "Do not claim a tool was used unless you emit the exact tool call.")
	return strings.Join(lines, "\n")
}

func describeRequiredFields(schema map[string]any) string {
	requiredAny, ok := schema["required"]
	if !ok {
		return ""
	}
	requiredItems, ok := requiredAny.([]any)
	if !ok || len(requiredItems) == 0 {
		return ""
	}
	fields := make([]string, 0, len(requiredItems))
	for _, item := range requiredItems {
		name := strings.TrimSpace(fmt.Sprint(item))
		if name != "" {
			fields = append(fields, name)
		}
	}
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, ", ")
}

func buildToolExample(name string, schema map[string]any) string {
	requiredAny, ok := schema["required"]
	if !ok {
		return name + "()"
	}
	requiredItems, ok := requiredAny.([]any)
	if !ok || len(requiredItems) == 0 {
		return name + "()"
	}
	properties, _ := schema["properties"].(map[string]any)
	args := make(map[string]any, len(requiredItems))
	for _, item := range requiredItems {
		field := strings.TrimSpace(fmt.Sprint(item))
		if field == "" {
			continue
		}
		args[field] = exampleValueForProperty(field, properties[field])
	}
	if len(args) == 0 {
		return name + "()"
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s(%s)", name, string(encoded))
}

func exampleValueForProperty(name string, property any) any {
	propMap, _ := property.(map[string]any)
	propType := strings.TrimSpace(fmt.Sprint(propMap["type"]))
	switch strings.ToLower(name) {
	case "command":
		return "ls"
	case "path":
		return "."
	}
	switch propType {
	case "integer", "number":
		return 1
	case "boolean":
		return true
	default:
		return "value"
	}
}

func (r *Runtime) handleCommand(req domain.RunRequest, text string) (bool, string) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "/skills list", "/skill list":
		return true, r.listSkills()
	case "/tools list", "/tool list":
		return true, r.listTools()
	case "/mcp status":
		return true, r.mcpStatus()
	case "/mcp tools":
		return true, r.mcpTools()
	}

	cmd, ok := profile.ParseCommand(text)
	if !ok {
		return false, ""
	}
	switch cmd.Kind {
	case profile.CommandProfileList:
		if r.profile == nil {
			return true, "profile service unavailable"
		}
		files := r.profile.AllowedFiles()
		if len(files) == 0 {
			return true, "no profile files are writable"
		}
		return true, "allowed profile files: " + strings.Join(files, ", ")
	case profile.CommandProfileShow:
		if r.profile == nil {
			return true, "profile service unavailable"
		}
		res, err := r.profile.Get(cmd.File)
		if err != nil {
			return true, "profile show failed: " + formatProfileCommandError(err)
		}
		if strings.TrimSpace(res.Content) == "" {
			return true, fmt.Sprintf("%s is empty", res.File)
		}
		return true, fmt.Sprintf("## %s\n%s", res.File, strings.TrimSpace(res.Content))
	case profile.CommandProfileDiff:
		if r.profile == nil {
			return true, "profile service unavailable"
		}
		candidate := strings.TrimSpace(cmd.Content)
		mode := profile.UpdateModeReplace
		if candidate == "" {
			p, ok := r.profile.LatestProposal(req.SessionKey, cmd.File)
			if !ok {
				return true, "profile diff requires content or an active proposal for this file"
			}
			candidate = p.Request.Content
			mode = p.Request.Mode
		}
		current, err := r.profile.Get(cmd.File)
		if err != nil {
			return true, "profile diff failed: " + formatProfileCommandError(err)
		}
		proposed, err := r.profile.Preview(profile.UpdateRequest{
			File:    cmd.File,
			Content: candidate,
			Mode:    mode,
			Reason:  "chat_command_diff",
		})
		if err != nil {
			return true, "profile diff failed: " + formatProfileCommandError(err)
		}
		diff := profile.RenderLineDiff(current.Content, proposed)
		return true, fmt.Sprintf("## Diff %s\n%s", current.File, diff)
	case profile.CommandProfileSet:
		if r.profile == nil {
			return true, "profile service unavailable"
		}
		if strings.TrimSpace(cmd.Content) == "" {
			return true, "profile set requires content"
		}
		res, err := r.profile.Update(profile.UpdateRequest{File: cmd.File, Content: cmd.Content, Mode: profile.UpdateModeReplace, Reason: "chat_command_set"})
		if err != nil {
			return true, "profile set failed: " + formatProfileCommandError(err)
		}
		return true, fmt.Sprintf("updated %s (%d bytes)", res.File, res.Bytes)
	case profile.CommandProfileAppend:
		if r.profile == nil {
			return true, "profile service unavailable"
		}
		if strings.TrimSpace(cmd.Content) == "" {
			return true, "profile append requires content"
		}
		res, err := r.profile.Update(profile.UpdateRequest{File: cmd.File, Content: cmd.Content, Mode: profile.UpdateModeAppend, Reason: "chat_command_append"})
		if err != nil {
			return true, "profile append failed: " + formatProfileCommandError(err)
		}
		return true, fmt.Sprintf("appended %s (%d bytes)", res.File, res.Bytes)
	case profile.CommandProfileApply:
		if r.profile == nil {
			return true, "profile service unavailable"
		}
		res, err := r.profile.ApplyProposal(req.SessionKey, cmd.ID)
		if err != nil {
			return true, "profile apply failed: " + formatProfileCommandError(err)
		}
		return true, fmt.Sprintf("proposal applied to %s (%d bytes)", res.File, res.Bytes)
	case profile.CommandOnboardStatus:
		if r.bootstrap == nil {
			return true, "bootstrap service unavailable"
		}
		st, err := r.bootstrap.Status()
		if err != nil {
			return true, "onboard status failed: " + err.Error()
		}
		return true, fmt.Sprintf("seeded=%v onboardingCompleted=%v bootstrapExists=%v", st.Seeded, st.OnboardingCompleted, st.BootstrapExists)
	case profile.CommandOnboardDone:
		if r.bootstrap == nil {
			return true, "bootstrap service unavailable"
		}
		st, err := r.bootstrap.Complete(true)
		if err != nil {
			return true, "onboard done failed: " + err.Error()
		}
		return true, fmt.Sprintf("onboarding completed=%v bootstrapRemoved=%v", st.OnboardingCompleted, !st.BootstrapExists)
	default:
		return false, ""
	}
}

func formatProfileCommandError(err error) string {
	if err == nil {
		return ""
	}
	switch strings.TrimSpace(err.Error()) {
	case "FORBIDDEN_FILE":
		return "file is not allowed. Use /profile list to see writable files."
	case "FILE_TOO_LARGE":
		return "content is too large for this file."
	case "PROPOSAL_INVALID":
		return "proposal is invalid or expired."
	case "INTERNAL":
		return "internal error while processing profile update."
	default:
		return err.Error()
	}
}

func (r *Runtime) maybeExecuteGeneratedToolCall(ctx context.Context, generated string) (string, string, *domain.ToolCall, bool) {
	name, input, found, err := r.parseToolCallFromGeneratedText(generated)
	if !found {
		return "", "", nil, false
	}
	if err != nil {
		return "", "tool error: " + err.Error(), nil, true
	}
	if !r.policy.Allowed(name) {
		return "", "tool blocked by policy", &domain.ToolCall{
			Name:   name,
			Input:  input,
			Status: "blocked",
		}, true
	}
	if r.tools == nil {
		return "", "tool error: tools registry is not configured", nil, true
	}
	res, execErr := r.tools.Execute(ctx, name, input)
	if execErr != nil {
		return "", "tool error: " + execErr.Error(), &domain.ToolCall{
			Name:   name,
			Input:  input,
			Error:  execErr.Error(),
			Status: "error",
		}, true
	}
	toolCall := &domain.ToolCall{
		Name:   name,
		Input:  input,
		Output: res.Data,
		Status: "success",
	}

	eventText := fmt.Sprintf("tool %s result: %+v", name, res.Data)
	return eventText, formatToolResultForUser(name, res.Data), toolCall, true
}

func (r *Runtime) parseToolCallFromGeneratedText(generated string) (string, map[string]any, bool, error) {
	if r.tools == nil {
		return "", nil, false, nil
	}
	registered := make(map[string]struct{}, len(r.tools.List()))
	for _, name := range r.tools.List() {
		registered[name] = struct{}{}
	}
	if len(registered) == 0 {
		return "", nil, false, nil
	}

	for _, match := range inlineCodePattern.FindAllStringSubmatch(generated, -1) {
		if len(match) != 2 {
			continue
		}
		if name, input, found, err := parseToolCallCandidate(match[1], registered); found || err != nil {
			return name, input, found, err
		}
	}

	lines := strings.Split(generated, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		if name, input, found, err := parseToolCallCandidate(line, registered); found || err != nil {
			return name, input, found, err
		}
	}
	return "", nil, false, nil
}

func parseToolCallCandidate(candidate string, registered map[string]struct{}) (string, map[string]any, bool, error) {
	candidate = strings.TrimSpace(strings.Trim(candidate, "`"))
	match := toolCallLinePattern.FindStringSubmatch(candidate)
	if len(match) != 3 {
		return "", nil, false, nil
	}
	name := match[1]
	if _, ok := registered[name]; !ok {
		return "", nil, false, nil
	}
	argsRaw := strings.TrimSpace(match[2])
	if argsRaw == "" {
		return name, map[string]any{}, true, nil
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(argsRaw), &input); err != nil {
		return "", nil, true, fmt.Errorf("invalid tool input JSON: %w", err)
	}
	return name, input, true, nil
}

func formatToolResultForUser(name string, data any) string {
	if formatted, ok := formatNamedToolResult(name, data); ok {
		return formatted
	}
	if formatted, ok := formatStructuredToolResult(data); ok {
		return formatted
	}
	encoded, marshalErr := json.Marshal(data)
	if marshalErr == nil {
		return string(encoded)
	}
	return fmt.Sprintf("%+v", data)
}

func formatNamedToolResult(name string, data any) (string, bool) {
	payload, ok := data.(map[string]any)
	if !ok {
		return "", false
	}
	switch name {
	case "time_now":
		if now, ok := payload["now"]; ok {
			return fmt.Sprintf("Server time now: %v", now), true
		}
	case "shell":
		return formatShellResult(payload)
	}
	return "", false
}

func formatShellResult(payload map[string]any) (string, bool) {
	stdout := strings.TrimSpace(asString(payload["stdout"]))
	stderr := strings.TrimSpace(asString(payload["stderr"]))
	exitCode, _ := asInt(payload["exitCode"])
	timedOut, _ := payload["timedOut"].(bool)

	switch {
	case stdout != "":
		return stdout, true
	case stderr != "":
		return stderr, true
	case timedOut:
		return "Command timed out.", true
	case exitCode != 0:
		return fmt.Sprintf("Command exited with status %d.", exitCode), true
	default:
		return "Command completed successfully.", true
	}
}

func formatStructuredToolResult(data any) (string, bool) {
	payload, ok := data.(map[string]any)
	if !ok {
		return "", false
	}
	if content, ok := payload["content"].([]any); ok {
		parts := make([]string, 0, len(content))
		for _, item := range content {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text := strings.TrimSpace(asString(itemMap["text"]))
			if text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n"), true
		}
	}
	if structured, ok := payload["structuredContent"].(map[string]any); ok {
		if text := strings.TrimSpace(asString(structured["content"])); text != "" {
			return text, true
		}
	}
	return "", false
}

func asString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	default:
		return fmt.Sprint(v)
	}
}

func asInt(v any) (int, bool) {
	switch value := v.(type) {
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	case float32:
		return int(value), true
	default:
		return 0, false
	}
}

func (r *Runtime) listSkills() string {
	if r.skills == nil {
		return "skills service unavailable"
	}
	var (
		loaded []skills.Skill
		err    error
	)
	if strings.TrimSpace(r.workspaceDir) != "" {
		loaded, err = r.skills.LoadAll(r.workspaceDir)
		if err != nil {
			return "skills list failed: " + err.Error()
		}
	} else {
		loaded = r.skills.LoadedSkills()
	}
	if len(loaded) == 0 {
		return "no skills found"
	}
	lines := make([]string, 0, len(loaded)+1)
	lines = append(lines, "skills:")
	for _, sk := range loaded {
		status := "enabled"
		if !sk.Enabled {
			status = "disabled"
			if strings.TrimSpace(sk.Reason) != "" {
				status += " (" + sk.Reason + ")"
			}
		}
		line := fmt.Sprintf("- %s [%s]", sk.Name, status)
		if desc := strings.TrimSpace(sk.Description); desc != "" {
			line += " - " + desc
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (r *Runtime) listTools() string {
	if r.tools == nil {
		return "no tools registered"
	}
	catalog := r.tools.Catalog()
	if len(catalog) == 0 {
		return "no tools registered"
	}
	lines := make([]string, 0, len(catalog)+1)
	lines = append(lines, "tools:")
	for _, tool := range catalog {
		status := "allowed"
		if !r.policy.Allowed(tool.Name) {
			status = "blocked"
		}
		line := fmt.Sprintf("- %s [%s]", tool.Name, status)
		if desc := strings.TrimSpace(tool.Description); desc != "" {
			line += " - " + desc
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (r *Runtime) mcpStatus() string {
	if r.mcp == nil {
		return "mcp runtime unavailable"
	}
	status := r.mcp.Status()
	if !status.Available {
		return "mcp runtime unavailable"
	}
	lines := []string{
		fmt.Sprintf("mcp: available=true prefix=%s tools=%d servers=%d", valueOrDash(status.ToolPrefix), status.RegisteredTools, len(status.Servers)),
	}
	if strings.TrimSpace(status.ConfigFile) != "" {
		lines = append(lines, "config: "+status.ConfigFile)
	}
	if len(status.Servers) == 0 {
		lines = append(lines, "servers: none")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "servers:")
	for _, server := range status.Servers {
		lines = append(lines, fmt.Sprintf("- %s [%s] tools=%d", server.Name, valueOrDash(server.Transport), server.ToolCount))
	}
	return strings.Join(lines, "\n")
}

func (r *Runtime) mcpTools() string {
	if r.mcp == nil {
		return "mcp runtime unavailable"
	}
	mappings := r.mcp.Tools()
	if len(mappings) == 0 {
		return "mcp tools: none"
	}
	lines := []string{"mcp tools:"}
	for _, mapping := range mappings {
		line := fmt.Sprintf("- %s -> %s.%s", mapping.LocalName, mapping.ServerName, mapping.RemoteName)
		if desc := strings.TrimSpace(mapping.Description); desc != "" {
			line += " - " + desc
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func valueOrDash(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	return s
}
