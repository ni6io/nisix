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

func (t *NowTool) Metadata() Metadata {
	return Metadata{
		Name:        t.Name(),
		Description: "Returns the current server timestamp in RFC3339 format.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"now": map[string]any{
					"type":   "string",
					"format": "date-time",
				},
			},
			"required": []string{"now"},
		},
	}
}

func (t *NowTool) Execute(_ context.Context, _ map[string]any) (Result, error) {
	return Result{Data: map[string]any{"now": time.Now().Format(time.RFC3339)}}, nil
}
