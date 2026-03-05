package tools

import (
	"context"
	"errors"
	"sort"
	"sync"
)

type Result struct {
	Data any
}

type Metadata struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"inputSchema,omitempty"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
}

type Tool interface {
	Name() string
	Execute(ctx context.Context, input map[string]any) (Result, error)
}

type describedTool interface {
	Metadata() Metadata
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Execute(ctx context.Context, name string, input map[string]any) (Result, error) {
	t, ok := r.Get(name)
	if !ok {
		return Result{}, errors.New("tools: tool not found")
	}
	return t.Execute(ctx, input)
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (r *Registry) Catalog() []Metadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.tools) == 0 {
		return []Metadata{}
	}

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Metadata, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		md := Metadata{
			Name:        name,
			InputSchema: map[string]any{"type": "object"},
		}
		if dt, ok := tool.(describedTool); ok {
			md = dt.Metadata()
			if md.Name == "" {
				md.Name = name
			}
		}
		if md.InputSchema == nil {
			md.InputSchema = map[string]any{"type": "object"}
		}
		out = append(out, md)
	}
	return out
}
