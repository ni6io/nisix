package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureLayoutSeedsFromTemplates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "main")
	templateDir := filepath.Join(filepath.Dir(root), "templates")

	if err := os.MkdirAll(filepath.Join(templateDir, "memory"), 0o755); err != nil {
		t.Fatalf("mkdir memory template: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(templateDir, "skills"), 0o755); err != nil {
		t.Fatalf("mkdir skills template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "IDENTITY.md"), []byte("name: Test"), 0o644); err != nil {
		t.Fatalf("write template identity: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "BOOTSTRAP.md"), []byte("# bootstrap"), 0o644); err != nil {
		t.Fatalf("write template bootstrap: %v", err)
	}

	if err := EnsureLayout(root, Options{
		BootstrapFromTemplates: true,
		TemplateDir:            templateDir,
	}); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(root, "IDENTITY.md"))
	if err != nil {
		t.Fatalf("read seeded identity: %v", err)
	}
	if !strings.Contains(string(b), "name: Test") {
		t.Fatalf("expected template content, got: %q", string(b))
	}
	st, err := GetStatus(root)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !st.Seeded || !st.BootstrapExists {
		t.Fatalf("expected seeded and bootstrapExists, got %+v", st)
	}
}

func TestEnsureLayoutDoesNotOverwriteExistingFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "main")
	templateDir := filepath.Join(filepath.Dir(root), "templates")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "IDENTITY.md"), []byte("name: Existing"), 0o644); err != nil {
		t.Fatalf("write existing identity: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "IDENTITY.md"), []byte("name: Template"), 0o644); err != nil {
		t.Fatalf("write template identity: %v", err)
	}

	if err := EnsureLayout(root, Options{
		BootstrapFromTemplates: true,
		TemplateDir:            templateDir,
	}); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(root, "IDENTITY.md"))
	if err != nil {
		t.Fatalf("read identity: %v", err)
	}
	if strings.Contains(string(b), "Template") {
		t.Fatalf("expected existing file to be preserved, got: %q", string(b))
	}
}

func TestEnsureLayoutCanDisableTemplateSeeding(t *testing.T) {
	root := filepath.Join(t.TempDir(), "main")
	templateDir := filepath.Join(filepath.Dir(root), "templates")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "IDENTITY.md"), []byte("name: Template"), 0o644); err != nil {
		t.Fatalf("write template identity: %v", err)
	}

	if err := EnsureLayout(root, Options{
		BootstrapFromTemplates: false,
		TemplateDir:            templateDir,
	}); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(root, "IDENTITY.md"))
	if err != nil {
		t.Fatalf("read identity: %v", err)
	}
	if strings.Contains(string(b), "Template") {
		t.Fatalf("expected no template content when disabled, got: %q", string(b))
	}
}

func TestCompleteOnboardingRemovesBootstrap(t *testing.T) {
	root := filepath.Join(t.TempDir(), "main")
	templateDir := filepath.Join(filepath.Dir(root), "templates")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "BOOTSTRAP.md"), []byte("boot"), 0o644); err != nil {
		t.Fatalf("write bootstrap template: %v", err)
	}
	if err := EnsureLayout(root, Options{
		BootstrapFromTemplates: true,
		TemplateDir:            templateDir,
	}); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}

	st, err := CompleteOnboarding(root, true)
	if err != nil {
		t.Fatalf("complete onboarding: %v", err)
	}
	if !st.OnboardingCompleted {
		t.Fatalf("expected onboarding completed")
	}
	if st.BootstrapExists {
		t.Fatalf("expected bootstrap removed")
	}
}
