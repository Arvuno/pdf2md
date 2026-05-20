package models

import (
	"testing"
)

func TestGet_DotsOCR(t *testing.T) {
	cfg, err := Get("dots-ocr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "dots-ocr" {
		t.Errorf("expected name 'dots-ocr', got %q", cfg.Name)
	}
	if cfg.HuggingFaceRepo != "rednote-hilab/dots.mocr" {
		t.Errorf("expected repo 'rednote-hilab/dots.mocr', got %q", cfg.HuggingFaceRepo)
	}
	if cfg.PostProcess != PostProcessJSONLayout {
		t.Errorf("expected PostProcessJSONLayout, got %v", cfg.PostProcess)
	}
	if cfg.ImagePromptTag == "" {
		t.Error("dots-ocr should have an ImagePromptTag")
	}
}

func TestGet_LogicsParsingV2(t *testing.T) {
	cfg, err := Get("logics-parsing-v2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "logics-parsing-v2" {
		t.Errorf("expected name 'logics-parsing-v2', got %q", cfg.Name)
	}
	if cfg.HuggingFaceRepo != "Logics-MLLM/Logics-Parsing-v2" {
		t.Errorf("expected repo 'Logics-MLLM/Logics-Parsing-v2', got %q", cfg.HuggingFaceRepo)
	}
	if cfg.PostProcess != PostProcessHTML {
		t.Errorf("expected PostProcessHTML, got %v", cfg.PostProcess)
	}
	if cfg.DefaultPrompt != "QwenVL HTML" {
		t.Errorf("expected prompt 'QwenVL HTML', got %q", cfg.DefaultPrompt)
	}
}

func TestGet_Unknown(t *testing.T) {
	_, err := Get("nonexistent-model")
	if err == nil {
		t.Error("expected error for unknown model")
	}
}

func TestAvailable(t *testing.T) {
	avail := Available()
	if avail == "" {
		t.Error("Available() should not be empty")
	}
	if !contains(avail, "dots-ocr") {
		t.Errorf("Available() should contain 'dots-ocr', got: %s", avail)
	}
	if !contains(avail, "logics-parsing-v2") {
		t.Errorf("Available() should contain 'logics-parsing-v2', got: %s", avail)
	}
}

func TestDefaultModel(t *testing.T) {
	if DefaultModel != "dots-ocr" {
		t.Errorf("expected default model 'dots-ocr', got %q", DefaultModel)
	}
	// Verify default model exists in registry
	_, err := Get(DefaultModel)
	if err != nil {
		t.Errorf("default model %q should exist in registry: %v", DefaultModel, err)
	}
}

func TestAllModelsHaveRequiredFields(t *testing.T) {
	for name, cfg := range map[string]Config{
		"dots-ocr":          MustGet("dots-ocr"),
		"logics-parsing-v2": MustGet("logics-parsing-v2"),
	} {
		if cfg.Name == "" {
			t.Errorf("%s: Name is empty", name)
		}
		if cfg.HuggingFaceRepo == "" {
			t.Errorf("%s: HuggingFaceRepo is empty", name)
		}
		if cfg.DefaultPrompt == "" {
			t.Errorf("%s: DefaultPrompt is empty", name)
		}
		if cfg.ServedModelName == "" {
			t.Errorf("%s: ServedModelName is empty", name)
		}
		if cfg.PostProcess == "" {
			t.Errorf("%s: PostProcess is empty", name)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
