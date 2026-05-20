// Package models defines supported VLM model configurations and provides a registry.
package models

import "fmt"

// PostProcessMode defines how model output is converted to Markdown.
type PostProcessMode string

const (
	// PostProcessJSONLayout: model returns JSON layout array (dots.mocr style)
	PostProcessJSONLayout PostProcessMode = "json_layout"
	// PostProcessHTML: model returns HTML (Logics-Parsing style)
	PostProcessHTML PostProcessMode = "html"
)

// Config holds all configuration for a supported VLM model.
type Config struct {
	// Name is the CLI-friendly identifier (e.g. "dots-ocr", "logics-parsing-v2").
	Name string
	// HuggingFaceRepo is the model repo on HuggingFace.
	HuggingFaceRepo string
	// DefaultPrompt is the prompt sent to the model for document parsing.
	DefaultPrompt string
	// ImagePromptTag is the special token sequence prepended before the prompt (empty if not needed).
	ImagePromptTag string
	// ServedModelName is the name registered in vLLM (--served-model-name).
	ServedModelName string
	// VLLMArgs are extra arguments passed to vllm serve (e.g. --trust-remote-code).
	VLLMArgs []string
	// PostProcess defines how to convert model output to Markdown.
	PostProcess PostProcessMode
	// Temperature for inference.
	Temperature float64
	// TopP for inference.
	TopP float64
	// MaxCompletionTokens for inference.
	MaxCompletionTokens int64
}

var registry = map[string]Config{
	"dots-ocr": {
		Name:            "dots-ocr",
		HuggingFaceRepo: "rednote-hilab/dots.mocr",
		DefaultPrompt: `Please output the layout information from the PDF image, including each layout element's bbox, its category, and the corresponding text content within the bbox.

1. Bbox format: [x1, y1, x2, y2]

2. Layout Categories: The possible categories are ['Caption', 'Footnote', 'Formula', 'List-item', 'Page-footer', 'Page-header', 'Picture', 'Section-header', 'Table', 'Text', 'Title'].

3. Text Extraction & Formatting Rules:
    - Picture: For the 'Picture' category, the text field should be omitted.
    - Formula: Format its text as LaTeX.
    - Table: Format its text as HTML.
    - All Others (Text, Title, etc.): Format their text as Markdown.

4. Constraints:
    - The output text must be the original text from the image, with no translation.
    - All layout elements must be sorted according to human reading order.

5. Final Output: The entire output must be a single JSON object.`,
		ImagePromptTag:     "<|img|><|imgpad|><|endofimg|>",
		ServedModelName:    "model",
		VLLMArgs:           []string{"--trust-remote-code"},
		PostProcess:        PostProcessJSONLayout,
		Temperature:        0.1,
		TopP:               0.9,
		MaxCompletionTokens: 32768,
	},
	"logics-parsing-v2": {
		Name:            "logics-parsing-v2",
		HuggingFaceRepo: "Logics-MLLM/Logics-Parsing-v2",
		DefaultPrompt:  "QwenVL HTML",
		ImagePromptTag:  "", // Qwen3-VL uses standard chat template, no special tag
		ServedModelName: "model",
		VLLMArgs:        []string{"--trust-remote-code", "--max-model-len", "32768"},
		PostProcess:     PostProcessHTML,
		Temperature:     0.1,
		TopP:            0.5,
		MaxCompletionTokens: 16384,
	},
}

// Get returns the model config by name. Returns error if not found.
func Get(name string) (Config, error) {
	cfg, ok := registry[name]
	if !ok {
		return Config{}, fmt.Errorf("unknown model %q; available models: %s", name, Available())
	}
	return cfg, nil
}

// MustGet returns the model config by name. Panics if not found.
func MustGet(name string) Config {
	cfg, err := Get(name)
	if err != nil {
		panic(err)
	}
	return cfg
}

// Available returns a comma-separated list of available model names.
func Available() string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	// Simple sort for deterministic output
	if len(names) == 2 {
		if names[0] > names[1] {
			names[0], names[1] = names[1], names[0]
		}
	}
	result := ""
	for i, n := range names {
		if i > 0 {
			result += ", "
		}
		result += n
	}
	return result
}

// DefaultModel is the default model used when --model is not specified.
const DefaultModel = "dots-ocr"
