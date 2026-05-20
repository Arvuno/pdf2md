// Package paddlelayout parses PaddleOCR-VL layout JSON and materializes layout block crops.
package paddlelayout

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Block is a single layout element detected by PaddleOCR-VL.
type Block struct {
	BBox      [4]float64 `json:"bbox"`
	Label     string     `json:"label"`
	Text      string     `json:"text,omitempty"`
	Order     int        `json:"order,omitempty"`
	CropImage string     `json:"crop_image,omitempty"`
}

// Result is the parsed page-level layout result.
type Result struct {
	Blocks []Block `json:"blocks"`
}

// FallbackPageBlock creates a single full-page layout block when the model
// returns OCR text but no parseable layout JSON.
func FallbackPageBlock(imagePath, text string) (Block, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return Block{}, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return Block{}, err
	}
	return Block{
		BBox:  [4]float64{0, 0, float64(cfg.Width), float64(cfg.Height)},
		Label: "page",
		Text:  strings.TrimSpace(text),
		Order: 1,
	}, nil
}

// Parse extracts layout blocks from a model response.
func Parse(raw string) (Result, error) {
	jsonText, err := extractJSON(raw)
	if err != nil {
		return Result{}, err
	}

	var object struct {
		Blocks []Block `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(jsonText), &object); err == nil && len(object.Blocks) > 0 {
		sortBlocks(object.Blocks)
		return Result{Blocks: object.Blocks}, nil
	}

	var blocks []Block
	if err := json.Unmarshal([]byte(jsonText), &blocks); err == nil && len(blocks) > 0 {
		sortBlocks(blocks)
		return Result{Blocks: blocks}, nil
	}

	return Result{}, fmt.Errorf("response does not contain layout blocks")
}

// SaveCrops crops all valid block bboxes from imagePath into outputDir.
func SaveCrops(imagePath, outputDir string, blocks []Block) ([]Block, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}

	out := make([]Block, len(blocks))
	copy(out, blocks)
	for i := range out {
		rect, ok := clampRect(out[i].BBox, bounds)
		if !ok {
			continue
		}
		sub, ok := cropImage(img, rect)
		if !ok {
			continue
		}

		name := fmt.Sprintf("block-%03d_%s.png", i+1, safeLabel(out[i].Label))
		path := filepath.Join(outputDir, name)
		if err := writePNG(path, sub); err != nil {
			return nil, err
		}
		out[i].CropImage = path
	}
	return out, nil
}

// ToMarkdown converts layout blocks to a single Markdown page.
func ToMarkdown(blocks []Block) string {
	var parts []string
	for _, b := range blocks {
		label := strings.ToLower(strings.TrimSpace(b.Label))
		text := strings.TrimSpace(b.Text)
		switch label {
		case "title", "doc_title":
			if text != "" {
				parts = append(parts, "# "+text)
			}
		case "paragraph_title", "section-header", "section_header":
			if text != "" {
				parts = append(parts, "## "+text)
			}
		case "formula":
			if text != "" {
				parts = append(parts, "$$\n"+text+"\n$$")
			}
		case "image", "picture", "chart", "seal":
			if b.CropImage != "" {
				parts = append(parts, fmt.Sprintf("![%s](%s)", label, filepath.ToSlash(b.CropImage)))
			} else if text != "" {
				parts = append(parts, text)
			}
		default:
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func extractJSON(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return trimmed, nil
	}

	fence := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")
	if m := fence.FindStringSubmatch(trimmed); len(m) == 2 {
		return strings.TrimSpace(m[1]), nil
	}

	startObj := strings.Index(trimmed, "{")
	startArr := strings.Index(trimmed, "[")
	start := -1
	endChar := byte('}')
	if startObj >= 0 && (startArr < 0 || startObj < startArr) {
		start = startObj
		endChar = '}'
	} else if startArr >= 0 {
		start = startArr
		endChar = ']'
	}
	if start < 0 {
		return "", fmt.Errorf("no JSON found")
	}
	end := strings.LastIndexByte(trimmed, endChar)
	if end <= start {
		return "", fmt.Errorf("incomplete JSON")
	}
	return strings.TrimSpace(trimmed[start : end+1]), nil
}

func sortBlocks(blocks []Block) {
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].Order != 0 || blocks[j].Order != 0 {
			return blocks[i].Order < blocks[j].Order
		}
		if blocks[i].BBox[1] == blocks[j].BBox[1] {
			return blocks[i].BBox[0] < blocks[j].BBox[0]
		}
		return blocks[i].BBox[1] < blocks[j].BBox[1]
	})
}

func clampRect(b [4]float64, bounds image.Rectangle) (image.Rectangle, bool) {
	x1, y1 := int(b[0]), int(b[1])
	x2, y2 := int(b[2]), int(b[3])
	if x2 < x1 {
		x1, x2 = x2, x1
	}
	if y2 < y1 {
		y1, y2 = y2, y1
	}
	if x1 < bounds.Min.X {
		x1 = bounds.Min.X
	}
	if y1 < bounds.Min.Y {
		y1 = bounds.Min.Y
	}
	if x2 > bounds.Max.X {
		x2 = bounds.Max.X
	}
	if y2 > bounds.Max.Y {
		y2 = bounds.Max.Y
	}
	if x2 <= x1 || y2 <= y1 {
		return image.Rectangle{}, false
	}
	return image.Rect(x1, y1, x2, y2), true
}

func cropImage(img image.Image, rect image.Rectangle) (image.Image, bool) {
	if s, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return s.SubImage(rect), true
	}
	return nil, false
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func safeLabel(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" {
		return "block"
	}
	re := regexp.MustCompile(`[^a-z0-9_-]+`)
	return re.ReplaceAllString(label, "_")
}
