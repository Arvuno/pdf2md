// Package root implements the vlmocr CLI command.
package root

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ninehills/pdf2md/vlmocr/pkg/docker"
	"github.com/ninehills/pdf2md/vlmocr/pkg/htmlmd"
	"github.com/ninehills/pdf2md/vlmocr/pkg/inference"
	"github.com/ninehills/pdf2md/vlmocr/pkg/layout"
	"github.com/ninehills/pdf2md/vlmocr/pkg/markdown"
	"github.com/ninehills/pdf2md/vlmocr/pkg/model"
	"github.com/ninehills/pdf2md/vlmocr/pkg/models"
	"github.com/ninehills/pdf2md/vlmocr/pkg/paddlelayout"
	"github.com/ninehills/pdf2md/vlmocr/pkg/pdf"
)

const (
	defaultDPI         = 200
	defaultPort        = 8000
	defaultConcurrency = 16
	defaultTimeout     = 10 * time.Minute
)

type Opts struct {
	ModelName     string
	ModelDir      string
	OutputDir     string
	DPI           int
	Port          int
	Concurrency   int
	VLLMImage     string
	LlamaCppImage string
	GPUDevices    string
	NoHeaders     bool
	Timeout       time.Duration
}

func NewCommand() *cobra.Command {
	opts := &Opts{}

	cmd := &cobra.Command{
		Use:   "vlmocr [flags] <input.pdf>",
		Short: "Convert PDF documents to Markdown using VLM OCR models",
		Long: `vlmocr is a CLI tool that converts PDF documents to Markdown using
Vision Language Models via local Docker inference servers.

Supported models:
  - dots-ocr          dots.mocr by RedNote (layout-aware OCR)
  - logics-parsing-v2 Logics-Parsing-v2 by Alibaba (HTML-structured parsing)
  - paddleocr-vl-1.5-gguf PaddleOCR-VL-1.5 GGUF via llama.cpp (ONNX layout + VLM OCR)

Output (default: current directory):
  - <name>.pdf_pages/ — per-page images + layout block crops
  - <name>.md         — combined Markdown
  - <name>.json       — detailed per-page data

Requirements:
  - Docker with GPU support (nvidia-docker)
  - ONNX Runtime (libonnxruntime.so) for paddleocr-vl-1.5-gguf layout detection

Model weights are automatically prepared under ./weights/<model>/ on first use.
Use --model-dir to specify a local model directory and skip download.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), args[0], opts)
		},
		SilenceUsage: true,
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.ModelName, "model", models.DefaultModel, "Model to use: "+models.Available())
	flags.StringVar(&opts.ModelDir, "model-dir", "", "Local model directory (skip auto-download)")
	flags.StringVarP(&opts.OutputDir, "output", "o", ".", "Output directory (default: current directory)")
	flags.IntVar(&opts.DPI, "dpi", defaultDPI, "DPI for PDF page rendering")
	flags.IntVar(&opts.Port, "port", defaultPort, "Host port for inference server")
	flags.IntVarP(&opts.Concurrency, "concurrency", "c", defaultConcurrency, "Max concurrent inference requests")
	flags.StringVar(&opts.VLLMImage, "vllm-image", docker.DefaultVLLMImage, "vLLM Docker image (vLLM models only)")
	flags.StringVar(&opts.LlamaCppImage, "llamacpp-image", docker.DefaultLlamaCppImage, "llama.cpp Docker image (llama.cpp models only)")
	flags.StringVar(&opts.GPUDevices, "gpu", "all", "GPU devices to use")
	flags.BoolVar(&opts.NoHeaders, "no-headers", false, "Skip page headers and footers")
	flags.DurationVar(&opts.Timeout, "timeout", defaultTimeout, "Timeout for inference server startup")

	return cmd
}

func Execute() {
	cmd := NewCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context, inputPath string, opts *Opts) error {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	modelCfg, err := models.Get(opts.ModelName)
	if err != nil {
		return err
	}

	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("input file not found: %s", inputPath)
	}

	if err := docker.CheckDocker(); err != nil {
		return err
	}

	// Step 1: Resolve model directory
	var modelDir string
	if opts.ModelDir != "" {
		modelDir, err = filepath.Abs(opts.ModelDir)
		if err != nil {
			return fmt.Errorf("resolving model dir: %w", err)
		}
		fmt.Printf("=== Using local model: %s ===\n", modelDir)
	} else {
		fmt.Println("=== Step 1: Prepare model ===")
		modelDir, err = model.Download(model.DownloadConfig{
			RepoID:     modelCfg.HuggingFaceRepo,
			TargetDir:  filepath.Join("weights", opts.ModelName),
			ReadyFiles: modelReadyFiles(modelCfg),
		})
		if err != nil {
			return fmt.Errorf("downloading model: %w", err)
		}
	}

	// Step 1b: If paddle layout, download/init layout ONNX model
	var detector *layout.Detector
	if modelCfg.PostProcess == models.PostProcessPaddleLayout {
		fmt.Println("=== Step 1b: Prepare layout model ===")
		layoutDir, err := model.Download(model.DownloadConfig{
			RepoID:     layout.ModelRepoID,
			TargetDir:  filepath.Join("weights", "layout-model"),
			ReadyFiles: []string{layout.ModelFileName, layout.ConfigFileName},
		})
		if err != nil {
			return fmt.Errorf("downloading layout model: %w", err)
		}
		if err := layout.InitONNXRuntime(); err != nil {
			return fmt.Errorf("initializing ONNX Runtime: %w", err)
		}
		detector, err = layout.NewDetector(layoutDir)
		if err != nil {
			return fmt.Errorf("creating layout detector: %w", err)
		}
		defer detector.Destroy()
	}

	inputAbs, err := filepath.Abs(inputPath)
	if err != nil {
		return fmt.Errorf("resolving input path: %w", err)
	}
	baseName := filepath.Base(inputAbs)
	ext := filepath.Ext(baseName)
	nameWithoutExt := baseName[:len(baseName)-len(ext)]

	outputDir, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		return fmt.Errorf("resolving output dir: %w", err)
	}

	// Step 2: Start Docker container
	fmt.Printf("\n=== Step 2: Start %s Docker server ===\n", modelCfg.Runtime)
	containerName := docker.DefaultContainerName
	defer func() {
		fmt.Println("\n=== Cleanup: Stopping Docker container ===")
		docker.StopContainer(containerName)
	}()

	// Select Docker image based on runtime and CLI flags
	image := opts.VLLMImage
	switch modelCfg.Runtime {
	case models.RuntimeLlamaCpp:
		if opts.LlamaCppImage != "" {
			image = opts.LlamaCppImage
		}
	}
	if modelCfg.DockerImage != "" {
		image = modelCfg.DockerImage
	}

	containerCfg := docker.Config{
		Runtime:         string(modelCfg.Runtime),
		Image:           image,
		ModelPath:       modelDir,
		ContainerName:   containerName,
		Port:            opts.Port,
		GPUDevices:      opts.GPUDevices,
		TensorParallel:  1,
		GPUMemUtil:      0.9,
		ServedModelName: modelCfg.ServedModelName,
		VLLMArgs:        modelCfg.VLLMArgs,
		LlamaModelFile:  modelCfg.LlamaModelFile,
		LlamaMMProjFile: modelCfg.LlamaMMProjFile,
		LlamaArgs:       modelCfg.LlamaArgs,
	}

	if _, err := docker.StartContainer(containerCfg); err != nil {
		return fmt.Errorf("starting Docker container: %w", err)
	}

	if err := docker.WaitForReady(ctx, opts.Port, opts.Timeout); err != nil {
		return fmt.Errorf("waiting for inference server: %w", err)
	}

	// Step 3: Extract PDF pages
	fmt.Println("\n=== Step 3: Extract PDF pages ===")
	pagesDir := filepath.Join(outputDir, baseName+"_pages")

	pages, err := pdf.ExtractPages(inputPath, opts.DPI, pagesDir)
	if err != nil {
		return fmt.Errorf("extracting PDF pages: %w", err)
	}
	fmt.Printf("Extracted %d pages at %d DPI → %s\n", len(pages), opts.DPI, pagesDir)

	// Step 4: Run inference
	fmt.Println("\n=== Step 4: Run OCR inference ===")
	client := inference.NewClient(
		fmt.Sprintf("http://localhost:%d/v1", opts.Port),
		modelCfg,
		opts.Timeout,
	)

	var responses []string
	var pageMarkdowns []string
	var layoutBlocks [][]paddlelayout.Block
	var layoutErrors []string

	if modelCfg.PostProcess == models.PostProcessPaddleLayout {
		responses, pageMarkdowns, layoutBlocks, layoutErrors, err = parsePaddlePages(ctx, client, modelCfg, detector, pages, pagesDir, outputDir)
	} else {
		responses, err = client.ParseImages(ctx, pages, opts.Concurrency)
		if err == nil {
			pageMarkdowns, err = convertPages(modelCfg, responses, opts.NoHeaders)
		}
	}
	if err != nil {
		return fmt.Errorf("running inference: %w", err)
	}
	fmt.Printf("Inference complete for %d pages\n", len(responses))

	combined := markdown.CombinePages(pageMarkdowns)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	mdPath := filepath.Join(outputDir, nameWithoutExt+".md")
	if err := os.WriteFile(mdPath, []byte(combined), 0644); err != nil {
		return fmt.Errorf("writing markdown: %w", err)
	}
	fmt.Printf("Markdown: %s\n", mdPath)

	jsonPath := filepath.Join(outputDir, nameWithoutExt+".json")
	if err := writeDetailJSON(jsonPath, inputAbs, opts.ModelName, pages, responses, pageMarkdowns, layoutBlocks, layoutErrors); err != nil {
		fmt.Printf("Warning: writing JSON: %v\n", err)
	} else {
		fmt.Printf("JSON:     %s\n", jsonPath)
	}

	return nil
}

func convertPages(modelCfg models.Config, responses []string, noHeaders bool) ([]string, error) {
	pageMarkdowns := make([]string, 0, len(responses))
	for i, resp := range responses {
		var md string
		var err error
		switch modelCfg.PostProcess {
		case models.PostProcessHTML:
			md = htmlmd.Convert(resp)
		case models.PostProcessJSONLayout:
			md, err = markdown.Convert(resp, markdown.Options{SkipHeadersFooters: noHeaders})
			if err != nil {
				fmt.Printf("Warning: page %d: %v (using raw response)\n", i+1, err)
				md = resp
			}
		default:
			md = resp
		}
		pageMarkdowns = append(pageMarkdowns, md)
	}
	return pageMarkdowns, nil
}

func parsePaddlePages(ctx context.Context, client *inference.Client, modelCfg models.Config, detector *layout.Detector, pages []string, pagesDir, outputDir string) ([]string, []string, [][]paddlelayout.Block, []string, error) {
	// Pipeline: layout detection (producer) feeds VLM workers (consumers) via channel.
	// Layout for page N+1 runs while VLM processes page N blocks concurrently.
	type pageResult struct {
		idx    int
		blocks []paddlelayout.Block
		raw    []string
		mds    []string
		err    string
	}

	nPages := len(pages)
	resultCh := make(chan pageResult, nPages)

	// Producer: run layout detection sequentially, push results to channel.
	go func() {
		defer close(resultCh)
		for i, pagePath := range pages {
			detectedBlocks, layoutErr := detectPageLayout(detector, pagePath)
			if layoutErr == nil && len(detectedBlocks) > 0 {
				cropDir := filepath.Join(pagesDir, fmt.Sprintf("page-%d_blocks", i+1))
				bs, rs, md := processLayoutBlocks(ctx, client, pagePath, cropDir, detectedBlocks, outputDir)
				resultCh <- pageResult{idx: i, blocks: bs, raw: rs, mds: md}
			} else {
				resp, err := client.ParseImageWithPrompt(ctx, pagePath, "OCR:")
				if err != nil {
					resultCh <- pageResult{idx: i, err: fmt.Sprintf("page %d: %v", i+1, err)}
					continue
				}
				block, blockErr := paddlelayout.FallbackPageBlock(pagePath, resp)
				if blockErr != nil {
					resultCh <- pageResult{idx: i, err: fmt.Sprintf("page %d fallback: %v", i+1, blockErr)}
					continue
				}
				cropDir := filepath.Join(pagesDir, fmt.Sprintf("page-%d_blocks", i+1))
				blocks, cropErr := paddlelayout.SaveCrops(pagePath, cropDir, []paddlelayout.Block{block})
				if cropErr != nil {
					resultCh <- pageResult{idx: i, err: fmt.Sprintf("page %d crops: %v", i+1, cropErr)}
					continue
				}
				resultCh <- pageResult{
					idx:    i,
					blocks: blocks,
					raw:    []string{resp},
					mds:    []string{paddlelayout.ToMarkdown(blocks)},
					err: func() string {
						if layoutErr != nil {
							return layoutErr.Error()
						}
						return ""
					}(),
				}
			}
		}
	}()

	// Collect results in order.
	results := make([]pageResult, nPages)
	for r := range resultCh {
		results[r.idx] = r
	}

	// Assemble final output.
	responses := make([]string, nPages)
	pageMarkdowns := make([]string, nPages)
	layoutBlocks := make([][]paddlelayout.Block, nPages)
	layoutErrors := make([]string, nPages)
	for i, r := range results {
		if r.err != "" && len(r.raw) == 0 {
			return nil, nil, nil, nil, fmt.Errorf("%s", r.err)
		}
		responses[i] = strings.Join(r.raw, "\n---\n")
		pageMarkdowns[i] = strings.Join(r.mds, "\n\n")
		layoutBlocks[i] = r.blocks
		layoutErrors[i] = r.err
	}

	return responses, pageMarkdowns, layoutBlocks, layoutErrors, nil
}
func detectPageLayout(detector *layout.Detector, pagePath string) ([]layout.Block, error) {
	if detector == nil {
		return nil, fmt.Errorf("layout detector not initialized")
	}
	f, err := os.Open(pagePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return detector.Detect(img)
}

func processLayoutBlocks(ctx context.Context, client *inference.Client, pagePath, cropDir string, detected []layout.Block, outputDir string) ([]paddlelayout.Block, []string, []string) {
	var blocks []paddlelayout.Block
	for _, b := range detected {
		simplified := labelToSimplified(b.Label)
		blocks = append(blocks, paddlelayout.Block{
			BBox:  b.BBox,
			Label: simplified,
			Order: b.ReadOrder,
		})
	}
	blocks, _ = paddlelayout.SaveCrops(pagePath, cropDir, blocks)

	// Make crop paths relative to outputDir
	for i := range blocks {
		if blocks[i].CropImage != "" {
			rel, err := filepath.Rel(outputDir, blocks[i].CropImage)
			if err == nil {
				blocks[i].CropImage = rel
			}
		}
	}

	var responses []string
	var mds []string
	for i, block := range blocks {
		prompt := layout.LabelPrompts[detected[i].Label]
		if prompt == "" || block.CropImage == "" {
			continue
		}
		// Read back the absolute path for VLM call
		absPath := filepath.Join(outputDir, block.CropImage)
		md, err := client.ParseImageWithPrompt(ctx, absPath, prompt)
		if err != nil {
			fmt.Printf("Warning: page block %d (%s): %v\n", i+1, detected[i].Label, err)
			continue
		}
		responses = append(responses, md)
		simple := labelToSimplified(detected[i].Label)
		switch simple {
		case "formula":
			md = "$$\n" + md + "\n$$"
		}
		mds = append(mds, md)
	}
	return blocks, responses, mds
}

func labelToSimplified(fineLabel string) string {
	if s, ok := layout.MarkdownLabelMap[fineLabel]; ok {
		return s
	}
	return "text"
}

func modelReadyFiles(modelCfg models.Config) []string {
	if modelCfg.Runtime == models.RuntimeLlamaCpp {
		return []string{modelCfg.LlamaModelFile, modelCfg.LlamaMMProjFile}
	}
	return []string{"config.json"}
}

type pageDetail struct {
	Page         int                  `json:"page"`
	Image        string               `json:"image"`
	Response     string               `json:"raw_response"`
	Markdown     string               `json:"markdown"`
	LayoutBlocks []paddlelayout.Block `json:"layout_blocks,omitempty"`
	LayoutError  string               `json:"layout_error,omitempty"`
}

type detailJSON struct {
	Source  string       `json:"source"`
	Model   string       `json:"model"`
	Pages   int          `json:"pages"`
	Results []pageDetail `json:"results"`
}

func writeDetailJSON(jsonPath, source, modelName string, pageImages, responses, pageMarkdowns []string, layoutBlocks [][]paddlelayout.Block, layoutErrors []string) error {
	results := make([]pageDetail, len(responses))
	for i := range responses {
		results[i] = pageDetail{
			Page:     i + 1,
			Image:    pageImages[i],
			Response: responses[i],
			Markdown: pageMarkdowns[i],
		}
		if i < len(layoutBlocks) {
			results[i].LayoutBlocks = layoutBlocks[i]
		}
		if i < len(layoutErrors) {
			results[i].LayoutError = layoutErrors[i]
		}
	}
	data, err := json.MarshalIndent(detailJSON{
		Source:  source,
		Model:   modelName,
		Pages:   len(responses),
		Results: results,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(jsonPath, data, 0644)
}
