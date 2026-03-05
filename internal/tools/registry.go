package tools

import (
	"context"
	"errors"
	"sync"
)

type Result struct {
	Data any
}

type Tool interface {
	Name() string
	Execute(ctx context.Context, input map[string]any) (Result, error)
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
	return out
}
