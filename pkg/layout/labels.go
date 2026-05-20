package layout

import (
	"encoding/json"
	"os"
)

// Block is a detected layout element.
type Block struct {
	Label      string
	Confidence float32
	BBox       [4]float64
	ReadOrder  int
}

// ModelRepoID is the HuggingFace repository for the ONNX layout model.
const ModelRepoID = "alex-dinh/PP-DocLayoutV3-ONNX"

// ModelFileName is the ONNX model file to use.
const ModelFileName = "PP-DocLayoutV3.onnx"

// ConfigFileName is the JSON config with label list.
const ConfigFileName = "config.json"

// LabelPrompts maps PP-DocLayout detection labels to PaddleOCR-VL VLM prompts.
// Empty string means skip (don't send to VLM).
var LabelPrompts = map[string]string{
	"abstract":          "OCR:",
	"algorithm":         "OCR:",
	"aside_text":        "", // ignored
	"chart":             "Chart Recognition:",
	"content":           "OCR:",
	"display_formula":   "Formula Recognition:",
	"doc_title":         "OCR:",
	"figure_title":      "OCR:",
	"footer":            "", // ignored
	"footer_image":      "",
	"footnote":          "",
	"formula_number":    "", // ignored
	"header":            "", // ignored
	"header_image":      "",
	"image":             "", // skip, keep original image reference
	"inline_formula":    "Formula Recognition:",
	"number":            "", // ignored
	"paragraph_title":   "OCR:",
	"reference":         "OCR:",
	"reference_content": "OCR:",
	"seal":              "Seal Recognition:",
	"table":             "Table Recognition:",
	"text":              "OCR:",
	"vertical_text":     "OCR:",
	"vision_footnote":   "", // ignored
}

// MarkdownLabelMap maps fine-grained labels to simplified labels for Markdown.
var MarkdownLabelMap = map[string]string{
	"abstract":          "abstract",
	"algorithm":         "text",
	"aside_text":        "text",
	"chart":             "chart",
	"content":           "text",
	"display_formula":   "formula",
	"doc_title":         "title",
	"figure_title":      "title",
	"footer":            "text",
	"footer_image":      "image",
	"footnote":          "text",
	"formula_number":    "text",
	"header":            "text",
	"header_image":      "image",
	"image":             "image",
	"inline_formula":    "formula",
	"number":            "text",
	"paragraph_title":   "title",
	"reference":         "text",
	"reference_content": "text",
	"seal":              "seal",
	"table":             "table",
	"text":              "text",
	"vertical_text":     "text",
	"vision_footnote":   "text",
}

// loadLabels reads the label list from a PP-DocLayout config.json.
func loadLabels(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		LabelList []string `json:"label_list"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg.LabelList, nil
}
