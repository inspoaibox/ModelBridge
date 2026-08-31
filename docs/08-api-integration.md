# 客户 API 接入说明

## 基本配置

将官方 OpenAI SDK 的 `base_url` 指向：

```text
https://<gateway-domain>/v1
```

使用租户控制台创建的 API Token。Token 必须已经绑定一个可用分组，并且当前模型已经被该分组中的渠道映射；否则请求会被拒绝。

## OpenAI Chat Completions

同步请求：

```bash
curl https://<gateway-domain>/v1/chat/completions \
  -H "Authorization: Bearer <api-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<configured-model>",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

流式请求：

```json
{
  "model": "<configured-model>",
  "stream": true,
  "messages": [{"role": "user", "content": "请分段回答"}]
}
```

标准工具调用字段、`response_format`、`reasoning_effort`、结构化内容和图片内容块会按渠道协议转换。网关不会执行客户提供的工具，只负责将工具定义和模型返回的工具调用安全转发给客户端。

## Embeddings

```bash
curl https://<gateway-domain>/v1/embeddings \
  -H "Authorization: Bearer <api-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<configured-embedding-model>",
    "input": ["第一段文本", "第二段文本"]
  }'
```

OpenAI/Grok 使用 OpenAI-compatible Embeddings；Gemini 使用官方 `EmbedContent` SDK。所有请求仍然经过 Token 模型白名单、分组、网络白名单、限流、渠道兜底和账务流程。

## Anthropic Messages 与 Responses

`POST /v1/messages` 接受 Anthropic Messages 的文本、图片内容块、工具定义和工具结果，`stream: true` 返回 Anthropic SSE 事件。

`POST /v1/responses` 提供文本和基础内容块映射，并支持文本流式事件。完整 Responses 高级事件、服务端工具和异步任务语义尚未开放。

## 图片、音频与视频

媒体请求使用相同的 `Authorization: Bearer <下游令牌>`、分组模型白名单、IP/域名白名单和 `Idempotency-Key`。图片和音频接口保持 OpenAI 兼容格式，视频接口使用平台任务 ID，避免暴露上游任务标识。

```text
POST /v1/images/generations       application/json
POST /v1/images/edits             multipart/form-data
POST /v1/audio/transcriptions      multipart/form-data
POST /v1/audio/translations       multipart/form-data
POST /v1/audio/speech             application/json -> audio bytes
POST /v1/videos                   application/json -> platform video job
GET  /v1/videos/{videoID}         application/json
GET  /v1/videos/{videoID}/content video bytes
```

图片生成至少需要 `model` 和 `prompt`，`n` 默认为 1；图片编辑和音频接口的上传字段分别为 `image`、可选 `mask`，以及 `file`。视频创建至少需要 `model` 和 `prompt`，`seconds` 等厂商字段会转发到对应官方协议。OpenAI/Grok 使用 OpenAI 兼容媒体路径，Gemini 使用原生模型 API。

视频任务完成后才结算最终用量。上游返回 Usage 优先使用上游 Usage；无法获得 Usage 时仅使用能够从请求或文件中验证的计量数据，不会把未知用量写成 0。

## 计费与失败

- 请求开始时按当前价格版本和分组倍率预占余额。
- 上游成功后按返回的实际 usage 结算；父 Token 与缓存、推理、媒体明细按官方包含关系拆分，缓存和推理不会再次加到普通 Token 中。纯文本上游没有 usage 时才使用保守本地估算，并将 usage event 标为 `local_estimate`；多模态或工具请求没有可靠 usage 时不会按 0 元放行。
- 如果最终费用高于预占金额，系统仍按实际费用完整结算；预付余额可能暂时为负数并阻止后续预占，直到管理员充值，避免成功请求因估算不足而悬挂或漏账。
- 多模态或工具请求在上游成功但无法取得可靠 Usage 时会进入 `settlement_pending`，保留预占并等待管理员通过使用记录补录真实计量；不会自动释放或按 0 元放行。
- 上游可重试错误会在同一分组内按优先级、权重切换备用渠道；已经向客户端发送流式内容后不再切换，避免响应被拼接。
- `Idempotency-Key` 用于防止重试造成重复预占或重复结算。
- 同一请求切换备用渠道时复用同一笔预占；最终账单跟随成功渠道的厂商、模型映射和请求开始时生效的价格快照。
- `401/403` 表示 Token、权限或网络白名单问题；`402` 表示余额不足；`503` 表示价格、分组、凭据或渠道暂时不可用。

### 多计量价格

平台不是只按文本输入/输出 Token 计费。价格版本可以同时保存多个计量组件：

| 组件示例 | 单位 |
| --- | --- |
| `input_tokens`、`output_tokens`、`reasoning_tokens` | USD / 1M Token |
| `input_audio_tokens`、`output_image_tokens` | USD / 1M 对应媒体 Token |
| `input_images`、`output_images` | USD / image |
| `input_audio_seconds`、`output_video_seconds` | USD / second |
| `requests`、`queries`、`sessions`、`pages` | USD / 对应单位 |
| `file_search_calls_1k`、`computer_use_input_tokens_1k` | USD / 1K calls 或 Token |

每次结算会固化 `usage_metrics`、`charge_breakdown`、价格版本和分组倍率。Priority、Flex、Batch 和上下文阶梯价只在请求服务等级或用量区间匹配时生效。上游没有返回且无法安全估算的计量不会按免费处理，而是等待对账或拒绝结算。

## 当前边界

Anthropic 专用 Embeddings，以及完整 Responses 高级事件仍未开放。图片、视频和音频接口已经接入，但具体模型必须存在对应渠道映射，并且上游账户本身必须开通该模型能力；否则返回明确的 unsupported/invalid 错误，不会被静默降级成文本请求。
