package tools

import (
	"context"
	"testing"
)

type stubTool struct {
	name string
}

func (t stubTool) Name() string { return t.name }

func (t stubTool) Execute(_ context.Context, _ map[string]any) (Result, error) {
	return Result{}, nil
}

func TestRegistryCatalogUsesMetadataAndSorts(t *testing.T) {
	reg := NewRegistry()
	reg.Register(stubTool{name: "zeta"})
	reg.Register(NewNowTool())

	catalog := reg.Catalog()
	if len(catalog) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(catalog))
	}
	if catalog[0].Name != "time_now" || catalog[1].Name != "zeta" {
		t.Fatalf("unexpected catalog ordering: %#v", catalog)
	}
	if catalog[0].Description == "" {
		t.Fatalf("expected metadata description for time_now")
	}
	if catalog[0].OutputSchema == nil {
		t.Fatalf("expected metadata output schema for time_now")
	}
}

func TestRegistryCatalogProvidesDefaultInputSchema(t *testing.T) {
	reg := NewRegistry()
	reg.Register(stubTool{name: "basic"})

	catalog := reg.Catalog()
	if len(catalog) != 1 {
		t.Fatalf("expected one catalog entry, got %d", len(catalog))
	}
	if got := catalog[0].InputSchema["type"]; got != "object" {
		t.Fatalf("expected default object input schema, got %#v", got)
	}
}
