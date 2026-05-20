package docker

import (
	"testing"
)

func TestCheckDocker(t *testing.T) {
	err := CheckDocker()
	if err != nil {
		t.Logf("Docker not available (may be expected in CI): %v", err)
	}
}

func TestDefaultConstants(t *testing.T) {
	if DefaultVLLMImage == "" {
		t.Error("DefaultVLLMImage should not be empty")
	}
	if DefaultContainerName != "vlmocr-vllm" {
		t.Errorf("expected DefaultContainerName 'vlmocr-vllm', got %q", DefaultContainerName)
	}
}

func TestStopContainer_NonExistent(t *testing.T) {
	StopContainer("nonexistent-container-12345")
}

func TestConfig_HasVLLMArgs(t *testing.T) {
	cfg := Config{
		VLLMArgs: []string{"--trust-remote-code"},
	}
	if len(cfg.VLLMArgs) != 1 {
		t.Errorf("expected 1 VLLM arg, got %d", len(cfg.VLLMArgs))
	}
}

func TestConfig_ServedModelName(t *testing.T) {
	cfg := Config{}
	if cfg.ServedModelName != "" {
		t.Error("expected empty ServedModelName by default")
	}
}
