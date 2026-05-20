// Package docker manages Docker container lifecycle for VLM inference servers.
package docker

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultVLLMImage is the official vLLM Docker image.
	DefaultVLLMImage = "vllm/vllm-openai:latest"
	// DefaultLlamaCppImage is the llama.cpp CUDA Docker image.
	DefaultLlamaCppImage = "ghcr.io/ggml-org/llama.cpp:full-cuda13"
	// DefaultContainerName is the default Docker container name.
	DefaultContainerName = "pdf2md-vllm"
)

// Config holds Docker container configuration.
type Config struct {
	Runtime         string   // vllm or llamacpp
	Image           string   // Docker image
	ModelPath       string   // Local path to model weights (mounted into container)
	ContainerName   string   // Docker container name
	Port            int      // Host port to expose
	GPUDevices      string   // GPU devices (default: "all")
	TensorParallel  int      // vLLM tensor parallel size (default: 1)
	GPUMemUtil      float64  // vLLM GPU memory utilization (default: 0.9)
	ServedModelName string   // Model name for API
	VLLMArgs        []string // Extra args for vllm serve
	LlamaModelFile  string   // llama.cpp main GGUF file under ModelPath
	LlamaMMProjFile string   // llama.cpp mmproj GGUF file under ModelPath
	LlamaArgs       []string // Extra args for llama-server
}

// CheckDocker verifies Docker is installed and running.
func CheckDocker() error {
	cmd := exec.Command("docker", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker is not available: %w\nPlease install Docker and ensure the daemon is running", err)
	}
	return nil
}

// StartContainer starts a Docker container for model inference.
// It returns the container ID.
func StartContainer(cfg Config) (string, error) {
	args, name, port := BuildRunArgs(cfg)

	// Stop and remove any existing container with the same name.
	StopContainer(name)

	fmt.Printf("Starting %s container %s on port %d...\n", cfg.Runtime, name, port)
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("starting container: %w\nOutput: %s", err, string(output))
	}

	containerID := strings.TrimSpace(string(output))
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}
	fmt.Printf("Container started: %s\n", containerID)
	return containerID, nil
}

// BuildRunArgs builds docker run arguments. Exposed for tests.
func BuildRunArgs(cfg Config) ([]string, string, int) {
	applyDefaults(&cfg)
	containerModelPath := "/model"

	args := []string{
		"run", "-d",
		"--name", cfg.ContainerName,
		"--runtime=nvidia",
		"-e", "NVIDIA_VISIBLE_DEVICES=" + cfg.GPUDevices,
		"--ipc=host",
		"-p", fmt.Sprintf("%d:%d", cfg.Port, containerPort(cfg.Runtime)),
		"-v", fmt.Sprintf("%s:%s:ro", cfg.ModelPath, containerModelPath),
		cfg.Image,
	}

	switch cfg.Runtime {
	case "llamacpp":
		args = append(args,
			"--server",
			"-m", filepath.ToSlash(filepath.Join(containerModelPath, cfg.LlamaModelFile)),
			"--mmproj", filepath.ToSlash(filepath.Join(containerModelPath, cfg.LlamaMMProjFile)),
			"--host", "0.0.0.0",
			"--port", strconv.Itoa(containerPort(cfg.Runtime)),
		)
		args = append(args, cfg.LlamaArgs...)
	default:
		args = append(args,
			containerModelPath,
			"--tensor-parallel-size", strconv.Itoa(cfg.TensorParallel),
			"--gpu-memory-utilization", fmt.Sprintf("%.2f", cfg.GPUMemUtil),
			"--chat-template-content-format", "string",
			"--served-model-name", cfg.ServedModelName,
			"--enforce-eager", // skip CUDA graph capture for faster startup
		)
		args = append(args, cfg.VLLMArgs...)
	}

	return args, cfg.ContainerName, cfg.Port
}

func applyDefaults(cfg *Config) {
	if cfg.Runtime == "" {
		cfg.Runtime = "vllm"
	}
	if cfg.Image == "" {
		if cfg.Runtime == "llamacpp" {
			cfg.Image = DefaultLlamaCppImage
		} else {
			cfg.Image = DefaultVLLMImage
		}
	}
	if cfg.ContainerName == "" {
		cfg.ContainerName = DefaultContainerName
	}
	if cfg.Port == 0 {
		cfg.Port = 8000
	}
	if cfg.GPUDevices == "" {
		cfg.GPUDevices = "all"
	}
	if cfg.TensorParallel == 0 {
		cfg.TensorParallel = 1
	}
	if cfg.GPUMemUtil == 0 {
		cfg.GPUMemUtil = 0.9
	}
	if cfg.ServedModelName == "" {
		cfg.ServedModelName = "model"
	}
}

func containerPort(runtime string) int {
	if runtime == "llamacpp" {
		return 8080
	}
	return 8000
}

// WaitForReady waits for the inference server to become ready.
func WaitForReady(ctx context.Context, port int, timeout time.Duration) error {
	if port == 0 {
		port = 8000
	}

	healthURL := fmt.Sprintf("http://localhost:%d/health", port)
	deadline := time.Now().Add(timeout)

	fmt.Printf("Waiting for inference server on port %d...\n", port)

	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Println("Inference server is ready!")
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for inference server after %s", timeout)
}

// StopContainer stops and removes a Docker container by name.
func StopContainer(containerName string) {
	if containerName == "" {
		containerName = DefaultContainerName
	}

	rmCmd := exec.Command("docker", "rm", "-f", containerName)
	rmCmd.Run() // Ignore errors (container might not exist)

	fmt.Printf("Container %s stopped and removed.\n", containerName)
}
