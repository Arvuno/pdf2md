// Package docker manages the vLLM Docker container lifecycle for VLM inference.
package docker

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultVLLMImage is the official vLLM Docker image.
	DefaultVLLMImage = "vllm/vllm-openai:latest"
	// DefaultContainerName is the default Docker container name.
	DefaultContainerName = "vlmocr-vllm"
)

// Config holds Docker container configuration.
type Config struct {
	Image           string   // Docker image (default: vllm/vllm-openai:latest)
	ModelPath       string   // Local path to model weights (mounted into container)
	ContainerName   string   // Docker container name
	Port            int      // Host port to expose (default: 8000)
	GPUDevices      string   // GPU devices (default: "all")
	TensorParallel  int      // Tensor parallel size (default: 1)
	GPUMemUtil      float64  // GPU memory utilization (default: 0.9)
	ServedModelName string   // Model name for API (--served-model-name)
	VLLMArgs        []string // Extra args for vllm serve (e.g. --trust-remote-code)
}

// CheckDocker verifies Docker is installed and running.
func CheckDocker() error {
	cmd := exec.Command("docker", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker is not available: %w\nPlease install Docker and ensure the daemon is running", err)
	}
	return nil
}

// StartContainer starts a vLLM Docker container for model inference.
// It returns the container ID.
func StartContainer(cfg Config) (string, error) {
	if cfg.Image == "" {
		cfg.Image = DefaultVLLMImage
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

	// Stop and remove any existing container with the same name
	StopContainer(cfg.ContainerName)

	containerModelPath := "/model"

	args := []string{
		"run", "-d",
		"--name", cfg.ContainerName,
		"--runtime=nvidia",
		"-e", "NVIDIA_VISIBLE_DEVICES=all",
		"--ipc=host",
		"-p", fmt.Sprintf("%d:8000", cfg.Port),
		"-v", fmt.Sprintf("%s:%s:ro", cfg.ModelPath, containerModelPath),
		cfg.Image,
		containerModelPath,
		"--tensor-parallel-size", strconv.Itoa(cfg.TensorParallel),
		"--gpu-memory-utilization", fmt.Sprintf("%.2f", cfg.GPUMemUtil),
		"--chat-template-content-format", "string",
		"--served-model-name", cfg.ServedModelName,
		"--enforce-eager", // skip CUDA graph capture for faster startup
	}

	// Append model-specific VLLM args (e.g. --trust-remote-code)
	args = append(args, cfg.VLLMArgs...)

	fmt.Printf("Starting vLLM container %s on port %d...\n", cfg.ContainerName, cfg.Port)
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

// WaitForReady waits for the vLLM server to become ready.
func WaitForReady(ctx context.Context, port int, timeout time.Duration) error {
	if port == 0 {
		port = 8000
	}

	healthURL := fmt.Sprintf("http://localhost:%d/health", port)
	deadline := time.Now().Add(timeout)

	fmt.Printf("Waiting for vLLM server on port %d...\n", port)

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
				fmt.Println("vLLM server is ready!")
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for vLLM server after %s", timeout)
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
