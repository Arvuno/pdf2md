# llama.cpp VL 并发性能调优记录

## 环境

- GPU: NVIDIA + CUDA
- 模型: PaddleOCR-VL-1.5-GGUF (0.9B params)
- 镜像: `ghcr.io/ggml-org/llama.cpp:full-cuda13`
- PDF: 10 页学术论文，每页约 19 个 layout block

## 调优历程

### v0: 基线

参数: `--temp 0 --ctx-size 131072 --n-gpu-layers 999`

| 指标 | 值 |
|------|-----|
| 总耗时 | ~39s |
| 成功率 | 部分失败 |
| layout 检测 | CPU（onnxruntime_go CGO） |

### v1: 并发槽位优化（失败）

尝试 `--ctx-size 8192 --parallel 16` → VL 图片提示溢出（512 tokens/slot 太小）

### v2: 修复上下文（当前）

```bash
--temp 0 --ctx-size 32768 --n-gpu-layers 999
--flash-attn on --parallel 8 --cont-batching
--batch-size 2048 --ubatch-size 512
-t 8 --threads-http 8 --no-webui
```

| 指标 | 基线 | v2 |
|------|------|-----|
| 总耗时 | ~39s | 36.6s |
| 成功率 | 部分失败 | 100% |

### v3: Pipeline 并行 + ONNX Docker GPU（当前）

- ONNX 布局检测：Docker 容器（CUDA 12.9 + cuDNN 9）
- Pipeline：layout 检测与 VLM 推理并行（Go channel producer/consumer）
- CGO_ENABLED=0：移除系统 onnxruntime 依赖

| 指标 | v2 | v3 |
|------|-----|-----|
| 总耗时 | 36.6s | **33s** |
| ONNX layout | CPU, ~150ms/page | **CUDA GPU, ~50ms/page** |
| 系统依赖 | onnxruntime-go CGO | **零依赖** |

## 最终参数

### llama.cpp 容器
```
--temp 0 --ctx-size 32768 --n-gpu-layers 999
--flash-attn on --parallel 8 --cont-batching
--batch-size 2048 --ubatch-size 512
-t 8 --threads-http 8 --no-webui
```

### ONNX 容器
- 镜像: `ghcr.io/ninehills/pdf2md-onnx:latest`
- 基础: `nvidia/cuda:12.9.2-cudnn-runtime-ubuntu24.04`
- CUDAExecutionProvider 生效
