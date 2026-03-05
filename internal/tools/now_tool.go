package tools

import (
	"context"
	"time"
)

type NowTool struct{}

func NewNowTool() *NowTool {
	return &NowTool{}
}

func (t *NowTool) Name() string {
	return "time_now"
}

func (t *NowTool) Execute(_ context.Context, _ map[string]any) (Result, error) {
	return Result{Data: map[string]any{"now": time.Now().Format(time.RFC3339)}}, nil
}
