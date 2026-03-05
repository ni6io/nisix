package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ni6io/nisix/internal/tools"
)

type FileConfig struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

type ServerConfig struct {
	Transport  string            `json:"transport"`
	Type       string            `json:"type"`
	Command    string            `json:"command"`
	Args       []string          `json:"args"`
	Env        map[string]string `json:"env"`
	Cwd        string            `json:"cwd"`
	URL        string            `json:"url"`
	MessageURL string            `json:"messageUrl"`
	Headers    map[string]string `json:"headers"`
	TimeoutSec int               `json:"timeoutSec"`
}

type Options struct {
	ToolPrefix string
	Logger     *slog.Logger
}

type Manager struct {
	clients       []*Client
	notifications chan Notification
	logger        *slog.Logger
	wg            sync.WaitGroup
	closeOnce     sync.Once
}

func RegisterFromFile(ctx context.Context, reg *tools.Registry, path string, opts Options) (*Manager, int, error) {
	if reg == nil {
		return nil, 0, fmt.Errorf("mcp: tools registry is nil")
	}
	if strings.TrimSpace(path) == "" {
		return nil, 0, fmt.Errorf("mcp: config path is empty")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("mcp: read config file: %w", err)
	}

	var cfg FileConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, 0, fmt.Errorf("mcp: parse config file: %w", err)
	}
	if len(cfg.MCPServers) == 0 {
		return nil, 0, nil
	}

	baseDir := filepath.Dir(path)
	prefix := sanitizeName(opts.ToolPrefix)
	if prefix == "" {
		prefix = "mcp"
	}

	existing := make(map[string]struct{})
	for _, name := range reg.List() {
		existing[name] = struct{}{}
	}

	serverNames := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)

	manager := &Manager{
		clients:       make([]*Client, 0, len(serverNames)),
		notifications: make(chan Notification, 256),
		logger:        opts.Logger,
	}
	registeredCount := 0
	for _, serverName := range serverNames {
		serverCfg := cfg.MCPServers[serverName]
		client, err := StartClient(ctx, serverName, baseDir, serverCfg, opts.Logger)
		if err != nil {
			_ = manager.Close()
			return nil, 0, err
		}
		remoteTools, err := client.ListTools(ctx)
		if err != nil {
			_ = client.Close()
			_ = manager.Close()
			return nil, 0, err
		}
		sort.SliceStable(remoteTools, func(i, j int) bool {
			return remoteTools[i].Name < remoteTools[j].Name
		})

		safeServer := sanitizeName(serverName)
		if safeServer == "" {
			safeServer = "server"
		}
		for _, rt := range remoteTools {
			toolName := uniqueToolName(existing, fmt.Sprintf("%s_%s_%s", prefix, safeServer, sanitizeName(rt.Name)))
			existing[toolName] = struct{}{}

			wrapped := &Tool{
				name:        toolName,
				serverName:  serverName,
				remoteName:  rt.Name,
				description: strings.TrimSpace(rt.Description),
				inputSchema: cloneMap(rt.InputSchema),
				client:      client,
			}
			reg.Register(wrapped)
			registeredCount++
		}
		manager.clients = append(manager.clients, client)
		manager.wg.Add(1)
		go manager.forwardClientNotifications(client)
	}

	return manager, registeredCount, nil
}

func (m *Manager) Notifications() <-chan Notification {
	if m == nil {
		return nil
	}
	return m.notifications
}

func (m *Manager) forwardClientNotifications(client *Client) {
	defer m.wg.Done()
	if client == nil {
		return
	}
	noteCh := client.Notifications()
	done := client.Done()
	for {
		select {
		case <-done:
			return
		case note := <-noteCh:
			select {
			case m.notifications <- note:
			default:
				if m.logger != nil {
					m.logger.Warn("mcp.manager.notification.drop", "server", note.Server, "method", note.Method)
				}
			}
		}
	}
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	var firstErr error
	m.closeOnce.Do(func() {
		for _, client := range m.clients {
			if err := client.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		m.wg.Wait()
		close(m.notifications)
	})
	return firstErr
}

type Tool struct {
	name        string
	serverName  string
	remoteName  string
	description string
	inputSchema map[string]any
	client      *Client
}

func (t *Tool) Name() string {
	return t.name
}

func (t *Tool) Metadata() tools.Metadata {
	desc := t.description
	if desc == "" {
		desc = fmt.Sprintf("MCP tool %s from server %s", t.remoteName, t.serverName)
	}
	schema := cloneMap(t.inputSchema)
	if schema == nil {
		schema = map[string]any{"type": "object"}
	}
	return tools.Metadata{
		Name:         t.name,
		Description:  desc,
		InputSchema:  schema,
		OutputSchema: map[string]any{"type": "object"},
	}
}

func (t *Tool) Execute(ctx context.Context, input map[string]any) (tools.Result, error) {
	if t.client == nil {
		return tools.Result{}, fmt.Errorf("mcp tool client is not available")
	}
	if input == nil {
		input = map[string]any{}
	}
	out, err := t.client.CallTool(ctx, t.remoteName, input)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Data: out}, nil
}

func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return ""
	}
	if out[0] < 'a' || out[0] > 'z' {
		out = "t_" + out
	}
	return out
}

func uniqueToolName(existing map[string]struct{}, base string) string {
	if base == "" {
		base = "mcp_tool"
	}
	if _, ok := existing[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		candidate := base + "_" + strconv.Itoa(i)
		if _, ok := existing[candidate]; !ok {
			return candidate
		}
	}
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
