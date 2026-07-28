package tui

import (
	"late/internal/config"
	"late/internal/pathutil"
	"os"
	"testing"
)

func TestModelPickerAppliesOrchestratorModelImmediately(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("APPDATA", configHome)

	lateConfigDir, err := pathutil.LateConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lateConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Models: []config.ModelSetting{
			{URL: "https://example.test/v1", Key: "secret", Model: "new-model"},
		},
	}
	model := NewModel(&mockOrchestrator{}, nil, cfg)
	model.Mode = ViewModelPicker
	model.ModelPickerAgents = []string{"orchestrator"}
	model.ModelPickerModels = []string{"default", "new-model"}
	model.ModelPickerAgentSelections = map[string]int{"orchestrator": 1}

	var applied config.ModelSetting
	model.ApplyOrchestratorModel = func(setting config.ModelSetting) {
		applied = setting
	}

	updated, _ := model.updateChat(mockKey{code: '\r', text: "enter"})

	if applied != cfg.Models[0] {
		t.Fatalf("applied model = %#v, want %#v", applied, cfg.Models[0])
	}
	if updated.ModelName != "new-model" {
		t.Fatalf("displayed model = %q, want %q", updated.ModelName, "new-model")
	}
	if updated.Mode != ViewChat {
		t.Fatalf("mode = %v, want ViewChat", updated.Mode)
	}
}
