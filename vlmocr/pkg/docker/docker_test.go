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
	if DefaultLlamaCppImage != "ghcr.io/ggml-org/llama.cpp:full-cuda13" {
		t.Errorf("unexpected DefaultLlamaCppImage: %s", DefaultLlamaCppImage)
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

func TestBuildRunArgs_LlamaCpp(t *testing.T) {
	args, name, port := BuildRunArgs(Config{
		Runtime:         "llamacpp",
		ModelPath:       "/tmp/model",
		Port:            8111,
		LlamaModelFile:  "PaddleOCR-VL-1.5.gguf",
		LlamaMMProjFile: "PaddleOCR-VL-1.5-mmproj.gguf",
		LlamaArgs:       []string{"--temp", "0"},
	})
	if name != DefaultContainerName {
		t.Errorf("unexpected container name: %s", name)
	}
	if port != 8111 {
		t.Errorf("unexpected port: %d", port)
	}
	joined := ""
	for _, arg := range args {
		joined += arg + " "
	}
	for _, want := range []string{"ghcr.io/ggml-org/llama.cpp:full-cuda13", "--server", "-m", "/model/PaddleOCR-VL-1.5.gguf", "--mmproj", "/model/PaddleOCR-VL-1.5-mmproj.gguf", "8111:8080"} {
		if !containsArg(joined, want) {
			t.Errorf("expected docker args to contain %q; args=%v", want, args)
		}
	}
}

func containsArg(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
