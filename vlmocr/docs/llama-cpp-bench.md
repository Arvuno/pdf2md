# llama.cpp VL 并发性能调优记录

## 基线 (commit f398507)

原始参数: `--temp 0 --ctx-size 131072 --n-gpu-layers 999`

| 指标 | 值 |
|------|-----|
| 总耗时 (real) | ~39s |
| 成功请求数 | 120 |
| 失败/警告 | 部分 block cropless |
| 10页 Markdown | 434行 / 58KB |

## 调优 v1: 优化并发参数 (commit 8e94001)

新增参数:
```
--ctx-size 8192     # 较小上下文 = 更多 slot
--flash-attn on     # Flash Attention
--parallel 16       # 16 并发 slot
--cont-batching     # 连续批处理
--batch-size 2048   # 大逻辑批次
--ubatch-size 512   # 物理批次
--cache-type-k q8_0 # KV K 量化
--cache-type-v q8_0 # KV V 量化
--threads-http 8    # HTTP 线程池
--no-webui          # 关闭 Web UI
```

**结果: FAIL** — `ctx-size 8192 / parallel 16 = 512 tokens/slot` 太小，VL 图片提示溢出。

## 调优 v2: 修复上下文 (当前)

参数调整:
```
--ctx-size 32768    # 32K 上下文 (4096/slot × 8)
--parallel 8        # 8 并发 slot
```

完整参数:
```
--temp 0 --ctx-size 32768 --n-gpu-layers 999
--flash-attn on --parallel 8 --cont-batching
--batch-size 2048 --ubatch-size 512
--cache-type-k q8_0 --cache-type-v q8_0
--threads-http 8 --no-webui
```

| 指标 | 基线 v0 | 调优 v2 | 改善 |
|------|---------|---------|------|
| 总耗时 (real) | ~39s | 36.6s | **-6%** |
| 成功率 | 部分失败 | 100% | ✅ |
| 10页 blocks | 19/page | 19/page | — |
| 总 crops | 120 | 120 | — |
| Markdown | 434行/58KB | 264行/44KB | 内容更精确 |
| concurrency=16 | — | 36.3s | 基本持平 |

## 分析

- **瓶颈不在 VLM 推理**，而在串行 layout 检测：每页 150ms × 10 = ~1.5s，实际 36s 中大部分是 VLM 等待
- context size 太大浪费 VRAM，太小 slot 不够。32K/8slot=4096 per slot 是 VL 的合理平衡点
- Flash Attention + KV 量化 (q8_0) 提升吞吐但未显著加速总时间（受串行页面处理限制）
- 下一步优化方向：**layout 检测与 VLM 流水线并行**（当前 page-0 layout → page-0 VLM → page-1 layout... 应该改为 page-0 layout → page-1 layout 期间 page-0 VLM 并发）
