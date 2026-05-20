# PaddleOCR-VL-1.5-GGUF 后端详细设计

## 目标

在现有 Go 版 `vlmocr` 中增加第三种模型：`paddleocr-vl-1.5-gguf`。

约束：

- 只改 Go 代码，不引入 Python、PaddlePaddle 运行时或其他语言脚本。
- 使用模型仓库：`PaddlePaddle/PaddleOCR-VL-1.5-GGUF`。
- 使用 Docker 镜像：`ghcr.io/ggml-org/llama.cpp:full-cuda13`。
- 保留现有 `-o/--output` 输出目录能力。
- 真实功能测试必须能跑通示例 PDF。
- PaddleOCR-VL 支持 layout 输出，所以输出中要保留 layout 信息，并能按 layout 裁图。

## 文档调研结论

PaddleOCR 官方文档强调：完整 PaddleOCR-VL pipeline 分两步：

1. layout analysis：检测元素、排序、裁图。
2. VLM recognition：对子图做识别，再按 reading order 合并。

官方的完整 pipeline 依赖 PaddleOCR/PaddlePaddle。由于本项目明确不引入新语言和依赖，本实现不会嵌入完整 PaddleOCR Python pipeline，而是在 Go 内实现一个轻量等价流程：

1. 使用 PDF 渲染得到页面图片。
2. 调用 llama.cpp 托管的 PaddleOCR-VL GGUF，让模型直接输出页面 layout JSON。
3. Go 解析 layout JSON。
4. 按 bbox 在原页面图上裁出元素图片。
5. 将 layout 块合成为 Markdown，并把 layout/crop 信息写入 JSON。
6. 若模型 layout JSON 不可解析，则退化为整页 `OCR:` 识别，保证端到端可用。

这不是官方 PaddleOCR pipeline 的完全复刻，但满足本项目约束：纯 Go、单二进制、Docker 后端、无 Python 依赖，并暴露 layout 与裁图产物。

## 后端抽象

扩展 `models.Config`：

- `Runtime`：`vllm` 或 `llamacpp`。
- `DockerImage`：模型默认 Docker 镜像。
- `ModelFile` / `MMProjFile`：llama.cpp 需要的 `.gguf` 文件名。
- `ServerArgs`：运行时参数。
- `PostProcess` 增加 `paddle_layout`。

现有模型继续走 vLLM：

- `dots-ocr`
- `logics-parsing-v2`

新模型走 llama.cpp：

- `paddleocr-vl-1.5-gguf`
- 镜像：`ghcr.io/ggml-org/llama.cpp:full-cuda13`
- 启动命令：`llama-server -m /model/PaddleOCR-VL-1.5.gguf --mmproj /model/PaddleOCR-VL-1.5-mmproj.gguf --host 0.0.0.0 --port 8080 --temp 0`

## 推理流程

### 普通模型

保持现状：

`page image -> OpenAI-compatible chat completion -> postprocess -> markdown/json`

### PaddleOCR-VL-GGUF

每页执行：

1. 发送 layout prompt：要求返回 JSON object：

```json
{
  "blocks": [
    {
      "bbox": [x1, y1, x2, y2],
      "label": "text|title|table|formula|image|chart|seal|...",
      "text": "markdown content",
      "order": 1
    }
  ]
}
```

2. Go 解析 JSON：
   - 支持从 markdown code fence 中提取 JSON。
   - 支持顶层 object 或 array。
   - 自动裁剪越界 bbox。
   - 按 `order` 排序；没有 order 时按 y/x 排序。

3. 裁图：
   - 输出到 `<pdf>.pdf_pages/page-N_blocks/`。
   - 文件名：`block-001_<label>.png`。
   - 每个 block 在 JSON 中记录 `crop_image`。

4. Markdown 合成：
   - `title/doc_title/paragraph_title` -> heading。
   - `table` 直接保留模型输出文本。
   - `formula` 用 LaTeX block 包裹。
   - `image/chart/seal` 插入图片引用。
   - 其他标签直接写文本。

5. 退化路径：
   - layout JSON 解析失败时，发送 `OCR:` prompt 做整页识别。
   - JSON 中记录 raw response 和 fallback markdown。

## 输出结构

对任意模型保持现有文件：

- `<name>.md`
- `<name>.json`
- `<name>.pdf_pages/`

PaddleOCR-VL-GGUF 额外输出：

- `<name>.pdf_pages/page-N_blocks/*.png`
- JSON 的每页记录中增加：
  - `layout_blocks`
  - `layout_error`（如有）

## 测试计划

单元测试：

- model registry 包含新模型，且 runtime/image/model file 正确。
- llama.cpp Docker 参数构造正确。
- layout JSON 提取、解析、排序、Markdown 合成正确。
- bbox 裁剪和 block 图片保存正确。
- CLI help 包含新模型。

真实功能测试：

```bash
./vlmocr --model paddleocr-vl-1.5-gguf --timeout 30m -o outputs/paddleocr-vl-1.5-gguf ../examples/2603.29199v1.pdf
```

验收：

- llama.cpp 容器能启动。
- 10 页 PDF 均完成处理。
- 输出 `.md`、`.json`、10 张页面图。
- JSON 中有 layout 或 fallback 记录。
- 如 layout 成功，存在 block crop 图片。
- `go build ./... && go vet ./... && go test ./... -count=1` 通过。
