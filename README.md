# pdf2md

将 PDF 文档转换为 Markdown 的命令行工具，支持三种视觉语言模型（VLM），通过本地 Docker 推理服务完成端到端转换。

**纯 Go 单二进制**，零系统依赖（Docker + GPU 除外）。

[![Release](https://img.shields.io/github/v/release/ninehills/pdf2md)](https://github.com/ninehills/pdf2md/releases)

## 支持的模型

| 模型 | 后端 | 说明 |
|------|------|------|
| `dots-ocr`（默认） | vLLM | RedNote dots.mocr，布局感知 OCR |
| `logics-parsing-v2` | vLLM | 阿里巴巴 Logics-Parsing-v2，HTML 结构化解析 |
| `paddleocr-vl-1.5-gguf` | llama.cpp + ONNX | PaddleOCR-VL-1.5 GGUF，两阶段 pipeline（布局检测 + 块识别） |

## 前置条件

- **Docker** + `nvidia-container-toolkit`（GPU 推理）
- 无需安装 Python、onnxruntime、CUDA 等

```bash
docker --version && nvidia-smi
```

## 安装

### 预编译二进制（推荐）

从 [Releases](https://github.com/ninehills/pdf2md/releases) 下载对应平台：

```bash
curl -sL https://github.com/ninehills/pdf2md/releases/download/v0.1/pdf2md_0.1_linux_amd64.tar.gz | tar xz
./pdf2md --help
```

支持 `linux/macos/windows × amd64/arm64` 共 6 个平台。

### 从源码编译

```bash
git clone https://github.com/ninehills/pdf2md && cd pdf2md
go build -o pdf2md .
```

## 使用

```bash
# 默认模型 dots-ocr
./pdf2md paper.pdf

# 指定模型
./pdf2md --model logics-parsing-v2 paper.pdf
./pdf2md --model paddleocr-vl-1.5-gguf paper.pdf

# 指定输出目录
./pdf2md -o ./output paper.pdf
```

### 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--model` | `dots-ocr` | 模型选择 |
| `--output`, `-o` | 当前目录 | 输出目录 |
| `--dpi` | `200` | PDF 渲染 DPI |
| `--port` | `8000` | 推理端口 |
| `--concurrency`, `-c` | `16` | 最大并发 |
| `--timeout` | `30m` | 服务启动超时 |
| `--vllm-image` | `vllm/vllm-openai:latest` | vLLM 镜像 |
| `--llamacpp-image` | `ghcr.io/ggml-org/llama.cpp:full-cuda13` | llama.cpp 镜像 |
| `--onnx-image` | `ghcr.io/ninehills/pdf2md-onnx:latest` | ONNX 布局检测镜像 |
| `--onnx-port` | `5001` | ONNX 服务端口 |
| `--model-dir` | `./weights/<model>/` | 本地模型目录 |

### PaddleOCR-VL（两阶段）

`paddleocr-vl-1.5-gguf` 使用独立的 ONNX 布局检测 Docker 容器，**无需本机安装 onnxruntime**：

```
PDF → 渲染页面
  → [ONNX 容器] PP-DocLayoutV3 布局检测 → bboxes + labels
  → 按 bbox 裁子图
  → [llama.cpp 容器] PaddleOCR-VL VLM 按块识别
  → 合并 → Markdown + JSON
```

```bash
./pdf2md --model paddleocr-vl-1.5-gguf paper.pdf
```

## 项目结构

```
pdf2md/
├── cmd/root/          # CLI 入口 + pipeline 编排
├── pkg/
│   ├── base64util/    # Base64 编解码
│   ├── docker/        # Docker 容器管理
│   ├── htmlmd/        # HTML → Markdown 转换
│   ├── inference/     # VLM HTTP 推理客户端
│   ├── layout/        # Label→Prompt 映射表
│   ├── layoutclient/  # ONNX Docker HTTP 客户端
│   ├── markdown/      # Markdown 合成/合并
│   ├── model/         # HF 模型下载
│   ├── models/        # 模型注册表
│   ├── paddlelayout/  # PaddleOCR layout 解析/裁图
│   └── pdf/           # PDF 页面渲染
├── docker/onnx/       # ONNX 容器镜像定义
├── docs/              # 设计文档
├── examples/          # 测试用 PDF
└── .github/workflows/ # CI/CD
```

## 开发

```bash
go test ./... -count=1    # 78 tests, 13 packages
go vet ./...              # 静态检查
```

## 致谢

- [go-fitz](https://github.com/gen2brain/go-fitz) — Go MuPDF 绑定
- [onnxruntime](https://onnxruntime.ai) — ONNX 运行时
- [vLLM](https://github.com/vllm-project/vllm) — VLM 推理引擎
- [llama.cpp](https://github.com/ggml-org/llama.cpp) — llama.cpp 推理引擎

## Contributing
PRs welcome!

<!-- Contributor: Arvuno - documentation update -->

