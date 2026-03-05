package skills

import "testing"

func TestExtractExplicitInvocations(t *testing.T) {
	got := ExtractExplicitInvocations("hello\n/skill alpha\n!skill beta")
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("unexpected explicit invocations: %#v", got)
	}
}

func TestSelectExplicitSkills(t *testing.T) {
	enabled := []Skill{{Name: "alpha", Enabled: true}, {Name: "beta", Enabled: true}}
	selected := selectExplicitSkills([]string{"beta", "alpha"}, enabled, 1)
	if len(selected) != 1 || selected[0].Name != "beta" {
		t.Fatalf("unexpected selected explicit skills: %#v", selected)
	}
}

func TestSelectAutoMatchedSkills(t *testing.T) {
	enabled := []Skill{
		{Name: "db", Description: "database migrations and schema changes", Enabled: true},
		{Name: "ui", Description: "frontend color and layout", Enabled: true},
	}
	selected := selectAutoMatchedSkills("please update database schema", enabled, 1)
	if len(selected) != 1 || selected[0].Name != "db" {
		t.Fatalf("unexpected selected auto skill: %#v", selected)
	}
}
