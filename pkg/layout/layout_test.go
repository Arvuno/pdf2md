package layout

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestLabelPrompts_Completeness(t *testing.T) {
	// Verify all 25 PP-DocLayout labels have a prompt mapping.
	cfgPath := filepath.Join("..", "..", "weights", "layout-model", ConfigFileName)
	if _, err := os.Stat(cfgPath); err != nil {
		t.Skip("layout model not downloaded, skipping label test")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		LabelList []string `json:"label_list"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	for _, label := range cfg.LabelList {
		prompt, ok := LabelPrompts[label]
		if !ok {
			t.Errorf("label %q has no entry in LabelPrompts", label)
		}
		t.Logf("  %-20s → %q", label, prompt)
	}
}

func TestPreprocessImage(t *testing.T) {
	// Create a small test image.
	img := createTestImage(100, 200)
	data := preprocessImage(img, 100, 200)
	expectedLen := 3 * ModelInputWidth * ModelInputHeight
	if len(data) != expectedLen {
		t.Errorf("expected %d floats, got %d", expectedLen, len(data))
	}
	// Verify data contains non-zero values (normalization applied).
	sum := float32(0)
	for _, v := range data {
		sum += v
	}
	if sum == 0 {
		t.Error("preprocessed data should not be all zeros")
	}
}

func TestPostprocessBoxes(t *testing.T) {
	labels := []string{"text", "title", "image", "table", "formula"}
	raw := []float32{
		// label, score, x1, y1, x2, y2, order
		0, 0.8, 100, 200, 300, 400, 1,
		1, 0.3, 50, 60, 70, 80, 2, // below threshold
		3, 0.9, 10, 10, 50, 50, 0, // before sorted by order
		2, 0.6, 500, 600, 700, 800, 5,
	}
	blocks := postprocessBoxes(raw, labels, 0.5, 0.5, 0.5)

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}
	// Should be sorted by reading order: order=0, order=1, order=5
	if blocks[0].Label != "table" {
		t.Errorf("expected first block 'table' (order=0), got %q", blocks[0].Label)
	}
	if blocks[1].Label != "text" {
		t.Errorf("expected second block 'text' (order=1), got %q", blocks[1].Label)
	}
	if blocks[2].Label != "image" {
		t.Errorf("expected third block 'image' (order=5), got %q", blocks[2].Label)
	}
	t.Logf("blocks: %+v", blocks)
}

func TestFindONNXRuntimeLib(t *testing.T) {
	path := findONNXRuntimeLib()
	if path == "" {
		t.Skip("onnxruntime lib not found on this system")
	}
	t.Logf("found: %s", path)
}

// createTestImage returns a simple RGBA image for testing.
func createTestImage(w, h int) *imageImpl {
	return &imageImpl{w: w, h: h}
}

type imageImpl struct {
	w, h int
}

func (m *imageImpl) ColorModel() color.Model { return nil }
func (m *imageImpl) Bounds() image.Rectangle { return image.Rect(0, 0, m.w, m.h) }
func (m *imageImpl) At(x, y int) color.Color { return color.RGBA{127, 128, 129, 255} }
