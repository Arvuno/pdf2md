// Package root implements the vlmocr CLI command.
package root

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ninehills/pdf2md/vlmocr/pkg/docker"
	"github.com/ninehills/pdf2md/vlmocr/pkg/htmlmd"
	"github.com/ninehills/pdf2md/vlmocr/pkg/inference"
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

// Opts holds all CLI options.
type Opts struct {
	ModelName   string
	ModelDir    string
	OutputDir   string
	DPI         int
	Port        int
	Concurrency int
	VLLMImage   string
	GPUDevices  string
	NoHeaders   bool
	Timeout     time.Duration
}

// NewCommand creates the root command.
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
  - paddleocr-vl-1.5-gguf PaddleOCR-VL-1.5 GGUF via llama.cpp (layout + OCR)

	Output (default: current directory):
  - <name>.pdf_pages/ — per-page images
  - <name>.md         — combined Markdown
  - <name>.json       — detailed per-page data

Requirements:
  - Docker with GPU support (nvidia-docker)

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
	flags.StringVar(&opts.GPUDevices, "gpu", "all", "GPU devices to use")
	flags.BoolVar(&opts.NoHeaders, "no-headers", false, "Skip page headers and footers")
	flags.DurationVar(&opts.Timeout, "timeout", defaultTimeout, "Timeout for vLLM server startup and inference")

	return cmd
}

// Execute runs the root command.
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
		// User specified a local model directory — skip download
		modelDir, err = filepath.Abs(opts.ModelDir)
		if err != nil {
			return fmt.Errorf("resolving model dir: %w", err)
		}
		fmt.Printf("=== Using local model: %s ===\n", modelDir)
	} else {
		// Auto-download/prepare model under local weights/ (skips if already prepared)
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

	image := opts.VLLMImage
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
		responses, pageMarkdowns, layoutBlocks, layoutErrors, err = parsePaddlePages(ctx, client, modelCfg, pages, pagesDir)
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

func modelReadyFiles(modelCfg models.Config) []string {
	if modelCfg.Runtime == models.RuntimeLlamaCpp {
		return []string{modelCfg.LlamaModelFile, modelCfg.LlamaMMProjFile}
	}
	return []string{"config.json"}
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

func parsePaddlePages(ctx context.Context, client *inference.Client, modelCfg models.Config, pages []string, pagesDir string) ([]string, []string, [][]paddlelayout.Block, []string, error) {
	responses := make([]string, 0, len(pages))
	pageMarkdowns := make([]string, 0, len(pages))
	layoutBlocks := make([][]paddlelayout.Block, 0, len(pages))
	layoutErrors := make([]string, 0, len(pages))

	for i, pagePath := range pages {
		resp, err := client.ParseImageWithPrompt(ctx, pagePath, modelCfg.DefaultPrompt)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("page %d layout: %w", i+1, err)
		}

		layout, err := paddlelayout.Parse(resp)
		if err != nil {
			fallbackPrompt := modelCfg.FallbackPrompt
			if fallbackPrompt == "" {
				fallbackPrompt = "OCR:"
			}
			fallback, fallbackErr := client.ParseImageWithPrompt(ctx, pagePath, fallbackPrompt)
			if fallbackErr != nil {
				return nil, nil, nil, nil, fmt.Errorf("page %d fallback OCR: %w (layout parse error: %v)", i+1, fallbackErr, err)
			}
			block, blockErr := paddlelayout.FallbackPageBlock(pagePath, fallback)
			if blockErr != nil {
				return nil, nil, nil, nil, fmt.Errorf("page %d fallback layout: %w", i+1, blockErr)
			}
			cropDir := filepath.Join(pagesDir, fmt.Sprintf("page-%d_blocks", i+1))
			blocks, cropErr := paddlelayout.SaveCrops(pagePath, cropDir, []paddlelayout.Block{block})
			if cropErr != nil {
				return nil, nil, nil, nil, fmt.Errorf("page %d fallback crops: %w", i+1, cropErr)
			}
			responses = append(responses, resp)
			pageMarkdowns = append(pageMarkdowns, paddlelayout.ToMarkdown(blocks))
			layoutBlocks = append(layoutBlocks, blocks)
			layoutErrors = append(layoutErrors, err.Error())
			fmt.Printf("Warning: page %d layout parse failed, used full-page layout fallback: %v\n", i+1, err)
			continue
		}

		cropDir := filepath.Join(pagesDir, fmt.Sprintf("page-%d_blocks", i+1))
		blocks, err := paddlelayout.SaveCrops(pagePath, cropDir, layout.Blocks)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("page %d crops: %w", i+1, err)
		}

		responses = append(responses, resp)
		pageMarkdowns = append(pageMarkdowns, paddlelayout.ToMarkdown(blocks))
		layoutBlocks = append(layoutBlocks, blocks)
		layoutErrors = append(layoutErrors, "")
	}
	return responses, pageMarkdowns, layoutBlocks, layoutErrors, nil
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
