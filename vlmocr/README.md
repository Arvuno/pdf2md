# vlmocr

将 PDF 文档转换为 Markdown 的命令行工具，支持多种视觉语言模型（VLM），通过本地 vLLM Docker 推理服务完成端到端转换。

**纯 Go 实现**，无需安装 Python 或 poppler-utils。

## 支持的模型

| 模型 | 说明 | HuggingFace 仓库 |
|------|------|-------------------|
| `dots-ocr` (默认) | RedNote dots.mocr，布局感知 OCR | `rednote-hilab/dots.mocr` |
| `logics-parsing-v2` | 阿里巴巴 Logics-Parsing-v2，HTML 结构化解析 | `Logics-MLLM/Logics-Parsing-v2` |

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
git clone <repo-url> && cd vlmocr
go build -o vlmocr .
```

### 第二步：使用

```bash
# 使用默认模型 dots-ocr（首次会自动下载/准备模型到 ./weights/dots-ocr/）
./vlmocr your_document.pdf

# 指定使用 logics-parsing-v2 模型
./vlmocr --model logics-parsing-v2 your_document.pdf
```

转换完成后，默认会在**当前目录**生成：

```
your_document.pdf_pages/   ← 每页图片
your_document.md           ← Markdown 文本
your_document.json         ← 详细 JSON（含每页原始输出）
```

也可以用 `-o/--output` 指定输出目录。

就这么简单！🎉

## 完整用法

```
vlmocr [flags] <input.pdf>
```

### 参数说明

| 参数 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--model` | | `dots-ocr` | 使用哪个模型 |
| `--model-dir` | | 自动使用 `./weights/<model>/` | 本地模型目录；指定后不下载 |
| `--output` | `-o` | 当前目录 | 输出目录 |
| `--dpi` | | `200` | PDF 转图片的 DPI |
| `--port` | | `8000` | vLLM 服务端口 |
| `--concurrency` | `-c` | `16` | 最大并发推理数 |
| `--vllm-image` | | `vllm/vllm-openai:latest` | vLLM Docker 镜像 |
| `--gpu` | | `all` | 使用哪些 GPU |
| `--no-headers` | | `false` | 去掉页眉页脚 |
| `--timeout` | | `10m` | 服务启动超时 |

### 使用示例

```bash
# 基本用法
./vlmocr paper.pdf

# 使用 logics-parsing-v2
./vlmocr --model logics-parsing-v2 paper.pdf

# 指定输出目录
./vlmocr -o ./output paper.pdf

# 自定义模型目录
./vlmocr --model-dir /data/models/dots-ocr paper.pdf

# 输出到指定目录
./vlmocr --model dots-ocr -o outputs/dots-ocr paper.pdf

# 低 DPI 快速预览
./vlmocr --dpi 100 paper.pdf
```

## 工作流程

```
PDF 文件
  ↓
1. 下载模型权重并准备到 `./weights/<model>/`（如未准备）
  ↓
2. 启动 vLLM Docker 推理服务
  ↓
3. PDF 页面转图片（内置 MuPDF）
  ↓
4. 并发调用 VLM 推理（OpenAI 兼容 API）
  ↓
5. 后处理：JSON/HTML → Markdown
  ↓
6. 关闭 Docker 容器
  ↓
输出：<name>.pdf_pages/ + <name>.md + <name>.json
```

## 项目结构

```
vlmocr/
├── main.go                    # 入口
├── cmd/root/root.go           # CLI 命令
├── pkg/
│   ├── models/                # 模型注册表
│   ├── docker/                # Docker 容器管理
│   ├── inference/             # OpenAI 兼容推理客户端
│   ├── markdown/              # JSON 布局 → Markdown（dots-ocr）
│   ├── htmlmd/                # HTML → Markdown（logics-parsing-v2）
│   ├── base64util/            # 图片 Base64 编码
│   ├── model/                 # 模型下载（go-huggingface）
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
- [cobra](https://github.com/spf13/cobra) — CLI 框架
