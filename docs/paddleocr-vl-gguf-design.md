# PaddleOCR-VL-1.5-GGUF 设计文档

## 目标

在 Go 版 `pdf2md` 中增加第三种模型：`paddleocr-vl-1.5-gguf`。

## 架构

两阶段 pipeline：

```
page-N.png
  ↓
[ONNX Docker 容器] PP-DocLayoutV3 布局检测
  → 25 类细粒度 bbox + label
  ↓
按 bbox 裁子图
  ↓
[llama.cpp Docker 容器] PaddleOCR-VL-1.5-GGUF VLM 识别
  → 按 label 选择 prompt (OCR:/Formula/Table/Chart/Seal Recognition:)
  ↓
按 reading order 合并 → Markdown + JSON
```

## 后端

- **布局检测**: ONNX PP-DocLayoutV3
  - 镜像: `ghcr.io/ninehills/pdf2md-onnx:latest`（`nvidia/cuda:12.9.2-cudnn-runtime-ubuntu24.04`）
  - 模型: `alex-dinh/PP-DocLayoutV3-ONNX` → 自动下载到 `weights/layout-model/`
  - 接口: Flask HTTP `/detect` API（POST base64 image → JSON blocks）

- **VLM 推理**: llama.cpp
  - 镜像: `ghcr.io/ggml-org/llama.cpp:full-cuda13`
  - 模型: `PaddlePaddle/PaddleOCR-VL-1.5-GGUF` → 自动下载到 `weights/paddleocr-vl-1.5-gguf/`
  - 接口: OpenAI-compatible HTTP API

## CLI 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--model` | `dots-ocr` | 选用模型 |
| `--llamacpp-image` | `ghcr.io/ggml-org/llama.cpp:full-cuda13` | llama.cpp Docker 镜像 |
| `--onnx-image` | `ghcr.io/ninehills/pdf2md-onnx:latest` | ONNX 布局检测 Docker 镜像 |
| `--onnx-port` | `5001` | ONNX 服务器主机端口 |

## 推理流程

每页执行：
1. **布局检测**: `POST /detect` → `[{Label, Confidence, BBox, ReadOrder}, ...]`
2. **裁图**: 按 bbox 在原页面上裁出子图 → `page-N_blocks/block-XXX_<label>.png`
3. **VLM 识别**: 根据 label 选择 prompt：
   - `text/title/paragraph_title/abstract/reference` → `OCR:`
   - `table` → `Table Recognition:`
   - `display_formula/inline_formula` → `Formula Recognition:`
   - `chart` → `Chart Recognition:`
   - `seal` → `Seal Recognition:`
   - `image/header/footer/footnote` → 跳过
4. **Markdown 合成**:
   - `title/doc_title/paragraph_title` → 标题
   - `formula` → LaTeX block
   - `table` → 保留原文本
   - 其他 → 直接写文本
5. **退化路径**: layout 解析失败时，整页 `OCR:` 识别保底

## 输出结构

```
outputs/paddleocr-vl-1.5-gguf/
├── 2603.29199v1.md           # 合并 Markdown
├── 2603.29199v1.json         # 完整 JSON（含每页 layout + responses）
└── 2603.29199v1.pdf_pages/
    ├── page-1.png            # 整页渲染
    ├── page-1_blocks/        # 布局块裁图
    │   ├── block-001_title.png
    │   ├── block-002_text.png
    │   └── ...
    └── page-2.png
```

## 测试

### 单元测试（78 tests, 13 packages）

```bash
go test ./... -count=1
```

### 真实功能测试

```bash
./pdf2md --model paddleocr-vl-1.5-gguf --timeout 3m \
  -o outputs/paddleocr-vl-1.5-gguf examples/2603.29199v1.pdf
```

结果：10 页，33s，零错误。ONNX CUDA 容器 + llama.cpp GPU 容器均正常启动/停止。
