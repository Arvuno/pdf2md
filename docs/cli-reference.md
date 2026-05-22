# CLI Reference

## Usage

```bash
pdf2md [flags] <input.pdf>
```

## Positional Arguments

| Argument | Description |
|----------|-------------|
| `input.pdf` | Path to the PDF file to convert |

## Flags

### Model Selection

| Flag | Default | Description |
|------|---------|-------------|
| `--model` | `dots-ocr` | Model selection. Available: `dots-ocr`, `logics-parsing-v2`, `paddleocr-vl-1.5-gguf` |

### Output

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | Current directory | Output directory for converted Markdown files |

### PDF Rendering

| Flag | Default | Description |
|------|---------|-------------|
| `--dpi` | `200` | DPI for PDF page rendering |

### Concurrency

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--concurrency` | `-c` | `16` | Maximum concurrent page processing |

### Timeout

| Flag | Default | Description |
|------|---------|-------------|
| `--timeout` | `30m` | Service startup timeout |

### Docker Images

| Flag | Default | Description |
|------|---------|-------------|
| `--vllm-image` | `vllm/vllm-openai:latest` | vLLM Docker image for `dots-ocr` and `logics-parsing-v2` models |
| `--llamacpp-image` | `ghcr.io/ggml-org/llama.cpp:full-cuda13` | llama.cpp Docker image for `paddleocr-vl-1.5-gguf` model |
| `--onnx-image` | `ghcr.io/ninehills/pdf2md-onnx:latest` | ONNX layout detection Docker image (used with `paddleocr-vl-1.5-gguf`) |
| `--onnx-port` | `5001` | Port for ONNX layout detection service |

### Model Storage

| Flag | Default | Description |
|------|---------|-------------|
| `--model-dir` | `./weights/<model>/` | Local model directory |

### Help and Version

| Flag | Description |
|------|-------------|
| `--help` | Show help information |
| `--version` | Show version number |

## Examples

### Basic Conversion

```bash
# Convert with default model (dots-ocr)
pdf2md paper.pdf
```

### Specify Model

```bash
# Use Logics-Parsing-v2 model
pdf2md --model logics-parsing-v2 paper.pdf

# Use PaddleOCR-VL model (two-stage pipeline)
pdf2md --model paddleocr-vl-1.5-gguf paper.pdf
```

### Output Directory

```bash
# Specify output directory
pdf2md -o ./output paper.pdf
```

### Performance Tuning

```bash
# Increase concurrency for faster multi-page processing
pdf2md -c 32 paper.pdf

# Higher DPI for better quality (slower)
pdf2md --dpi 300 paper.pdf
```

### Custom Docker Images

```bash
# Use specific vLLM image
pdf2md --vllm-image vllm/vllm-openai:v0.2.7 paper.pdf

# Use specific ONNX image
pdf2md --model paddleocr-vl-1.5-gguf --onnx-image ghcr.io/ninehills/pdf2md-onnx:v1.0 paper.pdf
```

### Custom Model Directory

```bash
# Use locally downloaded model
pdf2md --model-dir ./my-models/dots-ocr paper.pdf
```

## Exit Codes

| Code | Description |
|------|-------------|
| `0` | Success |
| `1` | Error (file not found, conversion failed, etc.) |
| `2` | Docker not available or misconfigured |