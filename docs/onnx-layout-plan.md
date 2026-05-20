# PaddleOCR-VL-GGUF 完整 Pipeline 实施计划

## 目标

用 ONNX PP-DocLayoutV3 实现布局检测，补全 PaddleOCR-VL 的两阶段 pipeline（layout → VLM recognition）。

## 架构概览

```
PDF 文件
  ↓
[Step 1] go-fitz 渲染页面 → page-N.png
  ↓
[Step 2] ONNX Docker 容器布局检测 → bboxes + labels
  ↓  (text/table/formula/chart/seal/image/title/...)
[Step 3] 按 bbox 裁出子图 → page-N_blocks/block-XXX_<label>.png
  ↓
[Step 4] 按 label 选择 VLM prompt 并发发给 llama.cpp:
  - text/title/paragraph_title → "OCR:"
  - table → "Table Recognition:"
  - formula → "Formula Recognition:"
  - chart → "Chart Recognition:"
  - seal → "Seal Recognition:"
  - image/picture → 跳过（保留原图引用）
  ↓
[Step 5] 按 reading order 合并 → Markdown + JSON
```

## 实现方式

### 布局检测: ONNX Docker 容器

- **镜像**: `ghcr.io/ninehills/pdf2md-onnx:latest`
- **基础镜像**: `nvidia/cuda:12.9.2-cudnn-runtime-ubuntu24.04`
- **模型**: `alex-dinh/PP-DocLayoutV3-ONNX` → 自动下载到 `weights/layout-model/`
- **接口**: Flask HTTP API (`POST /detect` with base64 image → JSON blocks)
- **服务启动**: Docker 容器，入口 `python3 /opt/server.py`，模型挂载到 `/model/`

### VLM 推理: llama.cpp Docker 容器

- **镜像**: `ghcr.io/ggml-org/llama.cpp:full-cuda13`
- **模型**: `PaddlePaddle/PaddleOCR-VL-1.5-GGUF` → 自动下载到 `weights/paddleocr-vl-1.5-gguf/`
- **接口**: OpenAI-compatible HTTP API
- **参数**: `--temp 0 --ctx-size 32768 --n-gpu-layers 999 --flash-attn on --parallel 8 --cont-batching --batch-size 2048 --ubatch-size 512 -t 8 --threads-http 8 --no-webui`

### Go 代码

- `pkg/layoutclient/client.go` — HTTP 客户端调用 ONNX `/detect` API
- `cmd/root/root.go` — pipeline 编排，自动启动/停止 ONNX + llama.cpp 容器
- `pkg/layout/labels.go` — Label→Prompt 映射表（25 个细粒度标签）
- `pkg/layout/layout_stub.go` — 非 CGO 构建的桩实现
- `pkg/paddlelayout/` — layout JSON 解析、裁图、Markdown 合成

## 零系统依赖

**无需本机安装** onnxruntime、CUDA、cuDNN——

- ONNX 布局检测走 Docker 容器（内置 CUDA 12 + cuDNN 9 + onnxruntime-gpu）
- llama.cpp 走 Docker 容器（内置 CUDA 13）
- Go 二进制 `CGO_ENABLED=0` 构建，全平台发布

## 文件结构

```
pdf2md/
├── pkg/layout/
│   ├── labels.go          # Label→Prompt 映射表（无 CGO 依赖）
│   ├── layout.go          # CGO ONNX 实现（//go:build cgo）
│   └── layout_stub.go     # 非 CGO 桩（//go:build !cgo）
├── pkg/layoutclient/
│   └── client.go          # ONNX Docker HTTP API 客户端
├── pkg/paddlelayout/
│   └── paddlelayout.go    # Layout JSON 解析、裁图、Markdown
├── docker/onnx/
│   ├── Dockerfile         # ONNX 容器镜像
│   └── server.py          # Flask ONNX 推理服务器
└── docs/
    └── onnx-layout-plan.md  # 本文档
```
