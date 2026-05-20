// Package layoutclient calls the ONNX layout detection server via HTTP.
package layoutclient

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	// DefaultONNXImage is the default Docker image for ONNX layout detection.
	DefaultONNXImage = "ghcr.io/ninehills/pdf2md-onnx:latest"
	// DefaultONNXPort is the default host port for the ONNX server.
	DefaultONNXPort = 5001
	// DefaultONNXContainerName is the default container name.
	DefaultONNXContainerName = "pdf2md-onnx"
)

// Block matches the layout.Block structure.
type Block struct {
	Label      string     `json:"Label"`
	Confidence float32    `json:"Confidence"`
	BBox       [4]float64 `json:"BBox"`
	ReadOrder  int        `json:"ReadOrder"`
}

// Client wraps the ONNX detection HTTP API.
type Client struct {
	baseURL string
	client  *http.Client
}

// NewClient creates a new layout detection HTTP client.
func NewClient(port int) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://localhost:%d", port),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Detect sends an image to the ONNX layout detection server and returns blocks.
func (c *Client) Detect(imagePath string) ([]Block, error) {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("reading image %s: %w", imagePath, err)
	}

	imgB64 := base64.StdEncoding.EncodeToString(data)

	body, err := json.Marshal(map[string]string{"image": imgB64})
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	resp, err := c.client.Post(c.baseURL+"/detect", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("calling ONNX server: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ONNX server returned %d: %s", resp.StatusCode, string(respBody))
	}

	var blocks []Block
	if err := json.Unmarshal(respBody, &blocks); err != nil {
		return nil, fmt.Errorf("parsing blocks: %w\nResponse: %s", err, string(respBody))
	}

	return blocks, nil
}

// WaitForReady waits for the ONNX server to be ready.
func (c *Client) WaitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthClient := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := healthClient.Get(c.baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for ONNX server after %s", timeout)
}
