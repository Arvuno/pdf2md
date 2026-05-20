# PaddleOCR-VL-GGUF 完整 Pipeline 实施计划

## 目标

用 ONNX 运行时实现 PP-DocLayout 布局检测，补全 PaddleOCR-VL 的两阶段 pipeline（layout → VLM recognition），全部在 Go 内完成。

## 架构概览

```
PDF 文件
  ↓
[Step 1] go-fitz 渲染页面 → page-N.png
  ↓
[Step 2] ONNX PP-DocLayout 模型 → bboxes + labels
  ↓  (text/table/formula/chart/seal/image/title...)
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

## 新增依赖

### Go module
- `github.com/yalue/onnxruntime_go` — ONNX Runtime Go 绑定

### 系统库
- `libonnxruntime.so` (Linux) 或 `libonnxruntime.dylib` (macOS)
  - 安装方式：`apt install libonnxruntime-dev` 或从 [Releases](https://github.com/microsoft/onnxruntime/releases) 下载
  - Docker 运行时：mount 宿主机 so 到容器，或由 ONNX 在宿主侧运行

### 模型文件
- PP-DocLayout ONNX 模型（~100-200MB）
  - 来源：PaddleOCR 模型 zoo → `paddle2onnx` 转换（一次性离线操作）
  - 或使用 PaddleOCR 已发布的 ONNX 模型
  - 下载到 `weights/layout-model/`

## 新增文件

```
pdf2md/
├── pkg/layout/
│   ├── layout.go          # ONNX 推理会话管理、布局检测核心逻辑
│   ├── layout_test.go     # 单元测试
│   └── labels.go          # label → VLM prompt 映射表
├── scripts/
│   └── convert_layout_model.sh  # paddle2onnx 转换脚本（一次性，文档性质）
└── docs/
    └── paddleocr-vl-gguf-design.md  # 更新设计文档
```

## 修改文件

### `pkg/models/models.go`
- 新增字段：
  ```go
  type Config struct {
      ...
      LayoutModelDir   string   // 本地布局模型目录 (e.g. "weights/layout-model")
      LabelPrompts     map[string]string  // label → VLM prompt 映射
  }
  ```
- `paddleocr-vl-1.5-gguf` 新增 `LabelPrompts` 和 `LayoutModelDir`

### `pkg/layout/layout.go`
```go
package layout

// Detector runs ONNX PP-DocLayout inference.
type Detector struct {
    session *ort.AdvancedSession
    input   *ort.Tensor[float32]
    output  *ort.Tensor[float32]
    labels  []string
    inputW  int  // 模型输入宽度
    inputH  int  // 模型输入高度
}

// NewDetector creates a detector from an ONNX model file.
func NewDetector(modelPath string, inputW, inputH int, labels []string) (*Detector, error)

// Detect runs layout detection on an image, returns blocks.
func (d *Detector) Detect(img image.Image) ([]Block, error)

// Block is a detected layout element.
type Block struct {
    Label      string
    Confidence float32
    BBox       [4]float64  // x1, y1, x2, y2
}
```

### `cmd/root/root.go` — `parsePaddlePages()` 重写
```go
func parsePaddlePages(...) {
    // 加载布局模型（首次使用）
    detector := initLayoutDetector(modelCfg)
    
    for i, pagePath := range pages {
        // 1. 布局检测
        img := loadImage(pagePath)
        blocks, err := detector.Detect(img)
        if err != nil { /* 退化到整页 OCR */ }
        
        // 2. 按 block 裁图 + 并发 VLM
        var blockMDs []string
        for _, block := range blocks {
            prompt := modelCfg.LabelPrompts[block.Label]
            if prompt == "" { continue }  // image 等跳过
            
            cropPath := saveCrop(img, block.BBox, block.Label)
            md, err := client.ParseImageWithPrompt(ctx, cropPath, prompt)
            blockMDs = append(blockMDs, md)
        }
        
        // 3. 合并
        pageMarkdowns[i] = strings.Join(blockMDs, "\n\n")
        layoutBlocks[i] = blocks  // 记录到 JSON
    }
}
```

## 布局模型标签与 VLM prompt 映射

```go
var LabelPrompts = map[string]string{
    "text":             "OCR:",
    "doc_title":        "OCR:",
    "paragraph_title":  "OCR:",
    "section_header":   "OCR:",
    "abstract":         "OCR:",
    "table":            "Table Recognition:",
    "formula":          "Formula Recognition:",
    "chart":            "Chart Recognition:",
    "seal":             "Seal Recognition:",
    "image":            "",  // 跳过，保留原图引用
    "picture":          "",
    "header":           "",  // 默认忽略
    "footer":           "",
    "footnote":         "",
    "number":           "",
}
```

## 退化路径

| 条件 | 行为 |
|------|------|
| ONNX 运行时不可用 | CLI 启动时警告，`paddleocr-vl-1.5-gguf` 退化为整页 OCR |
| 布局模型文件缺失 | 自动尝试下载到 `weights/layout-model/` |
| 布局检测失败（单页） | 整页 fallback layout + OCR: prompt |
| ONNX 推理出错 | 整页 fallback |

## 模型准备（一次性离线操作）

```bash
# 1. 安装 paddle2onnx
pip install paddle2onnx paddlepaddle

# 2. 下载 PP-DocLayout 模型
paddleocr get_model --model_name PP-DocLayout-L --save_dir ./model

# 3. 转换为 ONNX
paddle2onnx --model_dir ./model \
    --model_filename model.pdmodel \
    --params_filename model.pdparams \
    --save_file PP-DocLayout-L.onnx \
    --opset_version 16

# 4. 上传到可下载位置（如 HuggingFace）
```

或者直接使用 PaddleOCR 提供的预转换 ONNX 版本（如可用）。

## 实施步骤

### Phase 1: 基础 ONNX 集成
1. 添加 `onnxruntime_go` 依赖
2. 创建 `pkg/layout/` 包
3. 实现 `Detector` 的加载、推理、后处理
4. 单元测试（用已转换的 ONNX 模型 + 测试图片）

### Phase 2: Pipeline 集成
5. 更新 `models.Config` 添加 `LabelPrompts`
6. 重写 `parsePaddlePages` 支持 layout → 多 prompt VLM
7. 更新 `paddlelayout` 包适配新流程

### Phase 3: 端到端测试
8. 真实 PDF 完整流程测试
9. 退化路径测试
10. 对比报告

### Phase 4: 交付
11. 更新 README（ONNX 运行时安装说明）
12. 更新设计文档
13. `go build ./... && go vet ./... && go test ./...` 全绿
14. 提交

## 风险评估

| 风险 | 缓解措施 |
|------|----------|
| PP-DocLayout ONNX 模型不可直接获取 | 提供 paddle2onnx 转换脚本；或寻找 PaddleOCR 发布的 ONNX 版本 |
| ONNX Runtime 安装复杂 | Dockerfile 内预装；提供安装脚本；支持退化路径 |
| PP-DocLayout 输出格式与预期不同 | 先转换测试，适配后处理代码 |
| 多 prompt 导致总推理时间增加 | 并发发送（已有 errgroup 支持）；合理设置超时 |

## ONNX Runtime 安装（Linux）

```bash
# Ubuntu/Debian
wget https://github.com/microsoft/onnxruntime/releases/download/v1.20.0/onnxruntime-linux-x64-gpu-1.20.0.tgz
tar xzf onnxruntime-linux-x64-gpu-1.20.0.tgz
sudo cp onnxruntime-linux-x64-gpu-1.20.0/lib/libonnxruntime.so* /usr/local/lib/
sudo ldconfig
```

或 CPU only:
```bash
sudo apt install libonnxruntime-dev
```
