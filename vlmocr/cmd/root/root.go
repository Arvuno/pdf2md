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
Vision Language Models via a local vLLM Docker server.

Supported models:
  - dots-ocr          dots.mocr by RedNote (layout-aware OCR)
  - logics-parsing-v2 Logics-Parsing-v2 by Alibaba (HTML-structured parsing)

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
	flags.IntVar(&opts.Port, "port", defaultPort, "Port for vLLM server")
	flags.IntVarP(&opts.Concurrency, "concurrency", "c", defaultConcurrency, "Max concurrent inference requests")
	flags.StringVar(&opts.VLLMImage, "vllm-image", docker.DefaultVLLMImage, "vLLM Docker image")
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
			RepoID:    modelCfg.HuggingFaceRepo,
			TargetDir: filepath.Join("weights", opts.ModelName),
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
	fmt.Println("\n=== Step 2: Start vLLM Docker server ===")
	containerName := docker.DefaultContainerName
	defer func() {
		fmt.Println("\n=== Cleanup: Stopping Docker container ===")
		docker.StopContainer(containerName)
	}()

	containerCfg := docker.Config{
		Image:           opts.VLLMImage,
		ModelPath:       modelDir,
		ContainerName:   containerName,
		Port:            opts.Port,
		GPUDevices:      opts.GPUDevices,
		TensorParallel:  1,
		GPUMemUtil:      0.9,
		ServedModelName: modelCfg.ServedModelName,
		VLLMArgs:        modelCfg.VLLMArgs,
	}

	if _, err := docker.StartContainer(containerCfg); err != nil {
		return fmt.Errorf("starting Docker container: %w", err)
	}

	if err := docker.WaitForReady(ctx, opts.Port, opts.Timeout); err != nil {
		return fmt.Errorf("waiting for vLLM server: %w", err)
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

	responses, err := client.ParseImages(ctx, pages, opts.Concurrency)
	if err != nil {
		return fmt.Errorf("running inference: %w", err)
	}
	fmt.Printf("Inference complete for %d pages\n", len(responses))

	// Step 5: Convert to Markdown
	fmt.Println("\n=== Step 5: Convert to Markdown ===")
	var pageMarkdowns []string
	for i, resp := range responses {
		var md string
		switch modelCfg.PostProcess {
		case models.PostProcessHTML:
			md = htmlmd.Convert(resp)
		case models.PostProcessJSONLayout:
			md, err = markdown.Convert(resp, markdown.Options{
				SkipHeadersFooters: opts.NoHeaders,
			})
			if err != nil {
				fmt.Printf("Warning: page %d: %v (using raw response)\n", i+1, err)
				md = resp
			}
		default:
			md = resp
		}
		pageMarkdowns = append(pageMarkdowns, md)
	}

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
	if err := writeDetailJSON(jsonPath, inputAbs, opts.ModelName, pages, responses, pageMarkdowns); err != nil {
		fmt.Printf("Warning: writing JSON: %v\n", err)
	} else {
		fmt.Printf("JSON:     %s\n", jsonPath)
	}

	return nil
}

type pageDetail struct {
	Page     int    `json:"page"`
	Image    string `json:"image"`
	Response string `json:"raw_response"`
	Markdown string `json:"markdown"`
}

type detailJSON struct {
	Source  string       `json:"source"`
	Model   string       `json:"model"`
	Pages   int          `json:"pages"`
	Results []pageDetail `json:"results"`
}

func writeDetailJSON(jsonPath, source, modelName string, pageImages, responses, pageMarkdowns []string) error {
	results := make([]pageDetail, len(responses))
	for i := range responses {
		results[i] = pageDetail{
			Page:     i + 1,
			Image:    pageImages[i],
			Response: responses[i],
			Markdown: pageMarkdowns[i],
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
