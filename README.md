# pdf2md

将 PDF 文档转换为 Markdown 的命令行工具，支持多种视觉语言模型（VLM），通过本地 Docker 推理服务完成端到端转换。

**纯 Go 实现**，无需安装 Python 或 poppler-utils。

## 支持的模型

| 模型 | 说明 | HuggingFace 仓库 |
|------|------|-------------------|
| `dots-ocr` (默认) | RedNote dots.mocr，布局感知 OCR | `rednote-hilab/dots.mocr` |
| `logics-parsing-v2` | 阿里巴巴 Logics-Parsing-v2，HTML 结构化解析 | `Logics-MLLM/Logics-Parsing-v2` |
| `paddleocr-vl-1.5-gguf` | PaddleOCR-VL-1.5 GGUF，llama.cpp 后端，支持 layout JSON 和按块裁图 | `PaddlePaddle/PaddleOCR-VL-1.5-GGUF` |

### PaddleOCR-VL 特殊依赖

`paddleocr-vl-1.5-gguf` 使用 ONNX Runtime 进行页面布局检测，通过 Docker 容器自动启动，**无需本机安装**。

```bash
# 默认使用 ghcr.io/ninehills/pdf2md-onnx:latest（首次自动拉取）
./pdf2md --model paddleocr-vl-1.5-gguf paper.pdf

# 或指定自定义 ONNX 镜像
./pdf2md --model paddleocr-vl-1.5-gguf --onnx-image my-registry/onnx:latest paper.pdf
```

布局模型（PP-DocLayoutV3）会自动下载到 `./weights/layout-model/`，并通过 `-v` 挂载到 ONNX 容器中。
## 快速开始（小白版）

### 前置条件

只需要两样东西：
1. **Docker**（带 NVIDIA GPU 支持）
2. **Go 1.22+**（仅编译时需要）

```bash
# 检查 Docker
docker --version

# 检查 GPU
nvidia-smi
```

### 第一步：构建

```bash
git clone <repo-url> && cd pdf2md
go build -o pdf2md .
```

### 第二步：使用

```bash
# 使用默认模型 dots-ocr（首次会自动下载/准备模型到 ./weights/dots-ocr/）
./pdf2md your_document.pdf

# 指定使用 logics-parsing-v2 模型
./pdf2md --model logics-parsing-v2 your_document.pdf

# 使用 PaddleOCR-VL-1.5 GGUF（llama.cpp 后端，会额外输出 layout blocks 和裁图）
./pdf2md --model paddleocr-vl-1.5-gguf your_document.pdf
```

转换完成后，默认会在**当前目录**生成：

```
your_document.pdf_pages/   ← 每页图片
your_document.md           ← Markdown 文本
your_document.json         ← 详细 JSON（含每页原始输出）
```

也可以用 `-o/--output` 指定输出目录。

就这么简单！🎉

PaddleOCR-VL-GGUF 还会额外生成每页的布局块裁图：

```
your_document.pdf_pages/page-1_blocks/block-001_page.png
```

如果模型返回可解析的细粒度 layout JSON，会按元素 bbox 裁图；如果 llama.cpp 输出普通 OCR 文本，工具会生成覆盖整页的 fallback layout block，保证 JSON 中仍有 layout 结构和对应裁图。
## 完整用法

```
pdf2md [flags] <input.pdf>
```

### 参数说明

| 参数 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--model` | | `dots-ocr` | 使用哪个模型 |
| `--model-dir` | | 自动使用 `./weights/<model>/` | 本地模型目录；指定后不下载 |
| `--output` | `-o` | 当前目录 | 输出目录 |
| `--dpi` | | `200` | PDF 转图片的 DPI |
| `--port` | | `8000` | 推理服务宿主机端口 |
| `--concurrency` | `-c` | `16` | 最大并发推理数 |
| `--vllm-image` | | `vllm/vllm-openai:latest` | vLLM 模型使用的 Docker 镜像；llama.cpp 模型使用模型内置镜像 |
| `--llamacpp-image` | | `ghcr.io/ggml-org/llama.cpp:full-cuda13` | llama.cpp 模型使用的 Docker 镜像 |
| `--gpu` | | `all` | 使用哪些 GPU |
| `--no-headers` | | `false` | 去掉页眉页脚 |
| `--timeout` | | `10m` | 服务启动超时 |

### 使用示例

```bash
# 基本用法
./pdf2md paper.pdf

# 使用 logics-parsing-v2
./pdf2md --model logics-parsing-v2 paper.pdf

# 使用 PaddleOCR-VL-1.5 GGUF + llama.cpp
./pdf2md --model paddleocr-vl-1.5-gguf -o outputs/paddleocr-vl paper.pdf

# 指定输出目录
./pdf2md -o ./output paper.pdf

# 自定义模型目录
./pdf2md --model-dir /data/models/dots-ocr paper.pdf

# 输出到指定目录
./pdf2md --model dots-ocr -o outputs/dots-ocr paper.pdf

# 低 DPI 快速预览
./pdf2md --dpi 100 paper.pdf
```

## 工作流程

```
PDF 文件
  ↓
1. 下载模型权重并准备到 `./weights/<model>/`（如未准备）
  ↓
2. 启动 vLLM 或 llama.cpp Docker 推理服务
  ↓
3. PDF 页面转图片（内置 MuPDF）
  ↓
4. 并发调用 VLM 推理（OpenAI 兼容 API）
  ↓
5. 后处理：JSON/HTML/Paddle layout → Markdown
  ↓
6. 关闭 Docker 容器
  ↓
输出：<name>.pdf_pages/ + <name>.md + <name>.json
```

## 项目结构

```
pdf2md/
├── main.go                    # 入口
├── cmd/root/root.go           # CLI 命令
├── pkg/
│   ├── models/                # 模型注册表
│   ├── docker/                # Docker 容器管理
│   ├── inference/             # OpenAI 兼容推理客户端
│   ├── markdown/              # JSON 布局 → Markdown（dots-ocr）
│   ├── htmlmd/                # HTML → Markdown（logics-parsing-v2）
│   ├── paddlelayout/          # PaddleOCR-VL layout JSON → Markdown + 裁图
│   ├── base64util/            # 图片 Base64 编码
│   ├── model/                 # 模型下载/准备（go-huggingface → weights/）
│   └── pdf/                   # PDF 页面提取（go-fitz/MuPDF）
└── testdata/
```

## 添加新模型

在 `pkg/models/models.go` 的 `registry` 中添加新条目即可。

## 测试

```bash
go test ./... -v
```

## 技术栈

- [go-fitz](https://github.com/gen2brain/go-fitz) — PDF 渲染（MuPDF）
- [go-huggingface](https://github.com/gomlx/go-huggingface) — 模型下载
- [openai-go](https://github.com/openai/openai-go) — VLM 推理客户端
- [vllm](https://github.com/vllm-project/vllm) — 模型推理服务
- [llama.cpp](https://github.com/ggml-org/llama.cpp) — GGUF VLM 推理服务
- [cobra](https://github.com/spf13/cobra) — CLI 框架
