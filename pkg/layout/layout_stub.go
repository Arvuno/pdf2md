//go:build !cgo

package layout

import (
	"fmt"
	"image"
)

// InitONNXRuntime is a stub when CGO is disabled.
func InitONNXRuntime() error {
	return fmt.Errorf("ONNX Runtime is not available: build was compiled without CGO (CGO_ENABLED=0); paddleocr-vl-1.5-gguf requires onnxruntime")
}

// Detector stub.
type Detector struct{}

// NewDetector is a stub when CGO is disabled.
func NewDetector(modelDir string) (*Detector, error) {
	return nil, fmt.Errorf("ONNX Runtime is not available: build was compiled without CGO")
}

// Detect is a stub when CGO is disabled.
func (d *Detector) Detect(img image.Image) ([]Block, error) {
	return nil, fmt.Errorf("ONNX Runtime is not available: build was compiled without CGO")
}

// Destroy is a stub when CGO is disabled.
func (d *Detector) Destroy() {}
