// Package models defines supported VLM model configurations and provides a registry.
package models

import (
	"fmt"
	"sort"
)

// Runtime defines how a model is served.
type Runtime string

const (
	// RuntimeVLLM serves HuggingFace models through vLLM OpenAI-compatible API.
	RuntimeVLLM Runtime = "vllm"
	// RuntimeLlamaCpp serves GGUF models through llama.cpp OpenAI-compatible API.
	RuntimeLlamaCpp Runtime = "llamacpp"
)

// PostProcessMode defines how model output is converted to Markdown.
type PostProcessMode string

const (
	// PostProcessJSONLayout: model returns JSON layout array (dots.mocr style)
	PostProcessJSONLayout PostProcessMode = "json_layout"
	// PostProcessHTML: model returns HTML (Logics-Parsing style)
	PostProcessHTML PostProcessMode = "html"
	// PostProcessPaddleLayout: model returns PaddleOCR-VL layout JSON.
	PostProcessPaddleLayout PostProcessMode = "paddle_layout"
)

// Config holds all configuration for a supported VLM model.
type Config struct {
	// Name is the CLI-friendly identifier (e.g. "dots-ocr", "logics-parsing-v2").
	Name string
	// Runtime selects the serving backend.
	Runtime Runtime
	// HuggingFaceRepo is the model repo on HuggingFace.
	HuggingFaceRepo string
	// DefaultPrompt is the prompt sent to the model for document parsing.
	DefaultPrompt string
	// FallbackPrompt is used by models that support a simpler OCR fallback.
	FallbackPrompt string
	// ImagePromptTag is the special token sequence prepended before the prompt (empty if not needed).
	ImagePromptTag string
	// ServedModelName is the model name passed to OpenAI-compatible requests.
	ServedModelName string
	// DockerImage is the default Docker image for this model runtime.
	DockerImage string
	// VLLMArgs are extra arguments passed to vllm serve (e.g. --trust-remote-code).
	VLLMArgs []string
	// LlamaModelFile is the GGUF model file under the model directory.
	LlamaModelFile string
	// LlamaMMProjFile is the GGUF multimodal projector file under the model directory.
	LlamaMMProjFile string
	// LlamaArgs are extra arguments passed to llama-server.
	LlamaArgs []string
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
		Runtime:         RuntimeVLLM,
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
		ImagePromptTag:      "<|img|><|imgpad|><|endofimg|>",
		ServedModelName:     "model",
		VLLMArgs:            []string{"--trust-remote-code"},
		PostProcess:         PostProcessJSONLayout,
		Temperature:         0.1,
		TopP:                0.9,
		MaxCompletionTokens: 32768,
	},
	"logics-parsing-v2": {
		Name:                "logics-parsing-v2",
		Runtime:             RuntimeVLLM,
		HuggingFaceRepo:     "Logics-MLLM/Logics-Parsing-v2",
		DefaultPrompt:       "QwenVL HTML",
		ImagePromptTag:      "", // Qwen3-VL uses standard chat template, no special tag
		ServedModelName:     "model",
		VLLMArgs:            []string{"--trust-remote-code", "--max-model-len", "32768"},
		PostProcess:         PostProcessHTML,
		Temperature:         0.1,
		TopP:                0.5,
		MaxCompletionTokens: 16384,
	},
	"paddleocr-vl-1.5-gguf": {
		Name:            "paddleocr-vl-1.5-gguf",
		Runtime:         RuntimeLlamaCpp,
		HuggingFaceRepo: "PaddlePaddle/PaddleOCR-VL-1.5-GGUF",
		DefaultPrompt: `Analyze this document page and return ONLY valid JSON. Do not use markdown fences.
Return this schema:
{
  "blocks": [
    {"bbox": [x1, y1, x2, y2], "label": "text|title|table|formula|image|chart|seal|other", "text": "markdown content", "order": 1}
  ]
}
Coordinates must be pixel coordinates in the input image. Sort blocks by reading order. Preserve the original language. Use Markdown for text, HTML for tables, and LaTeX for formulas.`,
		FallbackPrompt:      "OCR:",
		ServedModelName:     "paddleocr-vl-1.5-gguf",
		DockerImage:         "ghcr.io/ggml-org/llama.cpp:full-cuda13",
		LlamaModelFile:      "PaddleOCR-VL-1.5.gguf",
		LlamaMMProjFile:     "PaddleOCR-VL-1.5-mmproj.gguf",
		LlamaArgs:           []string{"--temp", "0", "--ctx-size", "131072", "--n-gpu-layers", "999"},
		PostProcess:         PostProcessPaddleLayout,
		Temperature:         0,
		TopP:                1,
		MaxCompletionTokens: 8192,
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
	sort.Strings(names)

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
