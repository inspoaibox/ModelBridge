# 客户 API 接入说明

## 基本配置

将官方 OpenAI SDK 的 `base_url` 指向：

```text
https://<gateway-domain>/v1
```

使用租户控制台创建的 API Token。Token 必须已经绑定一个可用分组，并且当前模型已经被该分组中的渠道映射；否则请求会被拒绝。

## 控制台模型状态

登录租户控制台后，模型状态页面调用：

```text
GET /console/v1/tenants/{tenantID}/model-status
```

返回当前可见分组的模型路由摘要，包括 `normal`（正常）、`pending`（待观测）、`degraded`（部分可用）、`unavailable`（不可用）和 `disabled`（分组已停用）。活动分组可供租户选择；租户已有令牌绑定的停用分组也会保留显示，帮助解释令牌为何当前不可调用。路由数按该分组实际绑定的渠道模型映射统计；状态来自渠道启停、模型映射启停、自动熔断和真实请求成功/失败记录，不会主动请求上游，也不会返回 Base URL 或密钥。页面进入后默认每 15 秒刷新一次，并支持手动刷新。

模型状态是否开放由管理员系统设置的功能开关控制：

```text
GET /public/v1/features
```

该接口只返回 `model_status_enabled`。关闭时租户前端隐藏模型状态菜单，直接访问模型状态接口会返回 `404` 和 `MODEL_STATUS_DISABLED`。管理员可以在具有 `operations:read` 权限的“模型监控”页面调用：

```text
GET /admin/v1/model-status
```

管理员视图按分组列出全部已配置模型，并显示路由可用数、当前状态、最近延迟、近 7 天真实请求可用率和最近请求状态条。它只读取数据库中的渠道映射、熔断健康字段和请求记录，不主动探测上游。

## 管理员邮件设置

管理员系统设置包含“基础设置”“邮件设置”和“功能开关”三个独立区域。基础设置只负责网站名称、Logo 和 Favicon；邮件设置负责 SMTP 主机、端口、用户名、密码、发件人、TLS、公开访问地址、连接测试和测试邮件；功能开关中的邮件总开关默认关闭，关闭时系统不会发送邮件，也不会因为邮箱验证阻塞注册。开启邮件系统前必须完成有效 SMTP 配置。

邮件模板通过以下接口管理，所有写操作需要管理员权限和 Step-up MFA：

```text
GET    /admin/v1/settings/email
PUT    /admin/v1/settings/email
POST   /admin/v1/settings/email/test-connection
POST   /admin/v1/settings/email/test-message
GET    /admin/v1/settings/email/templates
POST   /admin/v1/settings/email/templates
PUT    /admin/v1/settings/email/templates/{templateID}
DELETE /admin/v1/settings/email/templates/{templateID}
GET    /admin/v1/settings/features
PUT    /admin/v1/settings/features
```

模板支持 `zh` 和 `en`，HTML 内容可使用 `{{site_name}}`、`{{user_email}}`、`{{verification_url}}`、`{{reset_url}}`、`{{recharge_url}}`、`{{balance}}`、`{{amount}}`、`{{event_time}}` 等变量。SMTP 密码只在服务端加密保存，任何读取接口只返回是否已配置。

## 注册与邮箱验证

公开注册是否要求邮箱验证由管理员“邮件总开关”和“邮箱验证码”开关共同决定。邮件关闭时注册直接创建 active 账号，不使用邮件系统；两项开关开启且 SMTP 可用时，注册创建 `pending` 账号并向注册邮箱发送一次性链接。邮箱验证链接会打开 `/#verify-email?token=...`，前端随后调用：

```text
POST /console/v1/auth/email/verify
{"token":"verify_..."}
```

验证令牌 30 分钟有效且只能使用一次。若邮件丢失，可以调用 `POST /console/v1/auth/email/resend` 请求重发；接口对未知邮箱返回相同的 accepted 语义，不暴露账号存在性。生产还必须在应用前配置 Captcha/Bot 防护和 WAF。

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

## Token 生命周期

API Token 只能由当前登录用户在租户控制台创建，创建响应返回一次完整密钥；管理员不能代发用户 Token。创建表单需要选择项目和路由分组，可选设置模型/网络白名单、过期时间和速率限制。Token 列表按 `tenant_id + created_by` 返回，其他成员和管理员都不能看到完整密钥。用户可以撤销自己创建的 Token；账号、租户成员或项目权限失效后，解析器会拒绝该 Token，相关项目权限降级也会撤销已有 Token。

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
