# 客户 API 接入说明

## 基本配置

管理员在“系统设置 -> 基础设置 -> API 对接终端”中配置客户可用的网关根地址。可以填写根地址或带 `/v1` 的地址，保存后系统统一按根地址保存，并自动派生两种客户端地址。客户可以在“控制台 -> API 令牌”页面直接复制：

```text
OpenAI 兼容地址：<gateway-root>/v1
Anthropic / Claude 地址：<gateway-root>
```

公开设置接口只返回已启用的终端名称和地址，不返回内部 ID 或上游密钥：

```text
GET /public/v1/settings
```

将官方 OpenAI SDK 的 `base_url` 指向终端的 OpenAI 兼容地址：

```text
https://<gateway-domain>/v1
```

使用租户控制台创建的 API Token。Token 必须已经绑定一个可用分组，并且当前模型已经被该分组中的渠道映射；否则请求会被拒绝。

对外网关会兼容以下路径形式，客户不需要手动处理 SDK 的路径拼接差异：

```text
/v1/chat/completions        标准 OpenAI 兼容形式
/chat/completions           根地址客户端形式
/v1/v1/chat/completions     客户端重复拼接 /v1 的容错形式
```

同样的兼容规则适用于模型、Responses、Embeddings、图片、音频、视频以及 Anthropic Messages 接口。服务端只为已定义的公开接口注册别名，所有别名仍经过同一个 API Token、模型权限、网络白名单、限流、渠道调度和计费流程。

## 控制台模型状态

登录租户控制台后，模型状态页面调用：

```text
GET /console/v1/tenants/{tenantID}/model-status
```

返回管理员已启用监控配置覆盖的分组模型路由摘要，包括 `normal`（正常）、`pending`（待观测）、`degraded`（部分可用）、`unavailable`（不可用）和 `disabled`（分组已停用）。监控选择“全部模型”时跟随分组当前启用的渠道模型映射；选择“指定模型”时，即使模型暂时没有可用渠道也会保留。活动分组可供租户选择；租户已有令牌绑定的停用分组也会保留显示，帮助解释令牌为何当前不可调用。路由数按该分组实际绑定的渠道模型映射统计；状态来自渠道启停、模型映射启停、自动熔断和真实请求成功/失败记录，不会主动请求上游，也不会返回 Base URL 或密钥。页面进入后默认每 15 秒刷新一次，并支持手动刷新。

模型状态是否开放由管理员系统设置的功能开关控制：

```text
GET /public/v1/features
```

该接口只返回 `model_status_enabled`。关闭时租户前端隐藏模型状态菜单，直接访问模型状态接口会返回 `404` 和 `MODEL_STATUS_DISABLED`。管理员可以在具有 `operations:read` 权限的“模型监控”页面调用：

```text
GET /admin/v1/model-status
```

管理员视图按已启用监控配置列出对应分组和模型，并显示路由可用数、当前状态、最近延迟、近 7 天真实请求可用率和按配置数量返回的最近请求状态条。选择全部模型时跟随分组当前启用映射；选择指定模型时，即使当前没有可用渠道也保留模型记录并显示不可用。被动模式只读取数据库中的渠道映射、熔断健康字段和请求记录；主动模式按配置周期使用最小请求探测上游。

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

## 企业认证

用户必须以当前租户的 active member 身份提交企业认证。表单使用 `multipart/form-data`：

```text
GET  /console/v1/tenants/{tenantID}/enterprise-verification
POST /console/v1/tenants/{tenantID}/enterprise-verification
```

提交字段为 `enterprise_name`、`unified_credit_code`、`license`、`bank_account_name`、`bank_name` 和 `bank_account`。营业执照支持 JPG、PNG、PDF，单文件最大 10 MB；服务端按文件内容校验类型并加密保存。用户查询只返回 `bank_account_masked`，不会返回完整银行账号。状态为 `not_submitted`、`pending`、`approved` 或 `rejected`；待审核不可重复提交，拒绝后可重新提交，已通过后不能重复提交。

管理员接口如下，写操作需要 `enterprise:update` 权限，审核还需要系统设置中为该操作启用的 Step-up MFA：

```text
GET  /admin/v1/enterprise-verifications?status=pending
GET  /admin/v1/enterprise-verifications/{verificationID}
GET  /admin/v1/enterprise-verifications/{verificationID}/license
POST /admin/v1/enterprise-verifications/{verificationID}/review
```

审核请求示例：

```json
{"status":"rejected","reason":"营业执照信息无法核验"}
```

列表接口仅返回银行账号脱敏值；管理员详情接口才返回完整账号，营业执照下载接口返回原始文件和 SHA-256 响应头。生产环境应限制管理员权限、记录访问审计并按企业资料保存期限执行数据治理。

## 在线充值与支付

用户充值接口使用当前租户的预付账户币种，金额按该币种的最小精度校验，必须带唯一 `Idempotency-Key`：

```text
POST /console/v1/tenants/{tenantID}/billing/recharge
GET  /console/v1/tenants/{tenantID}/billing/recharge/{orderID}
POST /console/v1/tenants/{tenantID}/billing/recharge/{orderID}/capture
```

创建请求：

```json
{"provider":"stripe","amount":"10.00","currency":"USD","return_url":"https://gateway.example.com/#console/billing"}
```

当前支付方式和配置字段：

| 支付方式 | 创建方式 | 必要的官方回调/验证 |
| --- | --- | --- |
| 微信支付 | Native 二维码 | 微信支付 API v3 平台证书验签、AES-256-GCM 解密、AppID/商户号校验 |
| 支付宝 | `alipay.trade.precreate` 当面付二维码 | RSA2、AppID、可选 seller ID、订单号和金额校验 |
| Stripe | Checkout Session | `Stripe-Signature` HMAC 验签和时间窗校验 |
| PayPal | Orders 创建，客户返回后 Capture | PayPal `verify-webhook-signature`，只接受 `PAYMENT.CAPTURE.COMPLETED` |

管理员在“系统设置 -> 支付设置”中分别维护并启用支付方式：

```text
GET /admin/v1/settings/payments
PUT /admin/v1/settings/payments/{provider}
```

支付配置的私钥、证书、API Key 和 Webhook Secret 使用 SecretBox 加密；读取接口仅返回非敏感字段及是否已配置。禁用方式不会删除已保存配置；开启前必须满足该方式的完整字段校验。微信支付和支付宝当面付只接受 CNY 租户账户；Stripe 和 PayPal 使用租户预付账户币种，平台不会在未配置汇率的情况下自动换算。

Stripe 不使用 `notify_url` 字段。后台会显示真实的 Webhook 地址，例如：

```text
https://gateway.example.com/payments/webhooks/stripe
```

管理员需要把这个地址添加到 Stripe Dashboard 的 Webhook endpoint，并把该 endpoint 的 signing secret 填入 `webhook_secret`。Stripe Checkout 的成功/取消返回地址由平台创建 Checkout Session 时自动传入，形式为 `https://gateway.example.com/?payment_order_id={order_id}&provider=stripe#console/billing` 和带 `cancelled=1` 的取消地址；它们只负责把用户带回充值页，不能作为入账依据。

Stripe 支付方式可以在后台按需勾选 `card`、`alipay`、`wechat_pay`。全部不勾选时平台启用 Stripe 的动态支付方式。Apple Pay 和 Google Pay 是卡支付钱包能力，不是需要单独提交的 Stripe `payment_method_types`；是否展示还取决于 Stripe 账户、客户设备、域名验证、币种和地区资格。当前平台的勾选配置是全局配置，不按国家保存独立规则；如需不同国家使用不同固定白名单，应在充值时收集国家并增加国家规则表和服务端匹配逻辑，不能仅靠前端隐藏选项实现。

支付回调由平台接收：

```text
POST /payments/webhooks/wechat
POST /payments/webhooks/alipay
POST /payments/webhooks/stripe
POST /payments/webhooks/paypal
```

支付回调不是前端传入成功状态的依据。服务端会校验平台签名、订单号、金额、币种、支付方式和订单有效期，验证成功后使用 `payment:credit:<order_id>` 写入不可变账务流水；重复回调不会重复增加余额。充值页返回后会按订单 ID 重新读取订单并轮询待支付订单，避免回跳后丢失状态。

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

如果使用 Anthropic SDK，请填写根地址并使用 Anthropic 的请求头：

```bash
curl https://<gateway-domain>/v1/messages \
  -H "x-api-key: <api-token>" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<configured-model>",
    "max_tokens": 1024,
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

API Token 只能由当前登录用户在租户控制台创建，创建响应返回一次完整密钥；管理员不能代发用户 Token。创建和编辑表单需要选择项目和路由分组，可选设置模型/网络白名单、过期时间、速率限制和消耗额度上限。`spend_limit` 默认为 `0`，表示不限制；设置为正数后，平台按该 Token 成功结算的客户侧费用累计到上限，后续付费请求返回 `402 TOKEN_SPEND_LIMIT_REACHED`。请求预留时会把同一 Token 尚未完成请求的预留金额计入，避免并发请求绕过上限；失败并释放预留的请求不会计入已消耗额度，已成功结算的请求会原子累加。Token 列表按 `tenant_id + created_by` 返回，其他成员和管理员都不能看到完整密钥。

用户可以对自己创建的 Token 执行以下操作：

| 操作 | 行为 |
| --- | --- |
| 编辑 | 修改名称、项目、分组、模型/网络白名单、过期时间、速率限制和消耗额度上限；不改变密钥本身 |
| 暂停 / 启用 | 在 `disabled` 与 `active` 之间切换；已过期或已终止的 Token 不能恢复 |
| 终止 | 将状态改为 `revoked`，永久失效且不能恢复 |
| 删除 | 软删除并终止 Token，只从用户列表隐藏，不破坏历史用量和财务记录 |
| 复制密钥 | 只有当前页面会话中刚创建的明文密钥可复制；历史 Token 只能看到脱敏前缀 |

对应接口为 `PUT /console/v1/tenants/{tenantID}/tokens/{tokenID}`、`POST .../pause`、`POST .../resume`、`POST .../terminate` 和 `DELETE /console/v1/tenants/{tenantID}/tokens/{tokenID}`。所有接口都由服务端同时校验当前会话的租户、创建者和项目权限，客户端不能通过请求体修改 `tenant_id`、`created_by` 或 Token 状态。账号、租户成员或项目权限失效后，解析器会拒绝该 Token，相关项目权限降级也会撤销已有 Token。

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

## 字节跳动/火山方舟 Seedance

管理员在“渠道管理 -> 新增渠道”选择“字节跳动/火山方舟官方协议 (Seedance)”，填写 Ark API Key，并将渠道模型映射到官方模型 ID：

```text
doubao-seedance-2-0-260128
doubao-seedance-2-5-260628
```

Base URL 可以填写以下任一形式，系统只在发送请求时补全，不会重复拼接：

```text
https://ark.cn-beijing.volces.com
https://ark.cn-beijing.volces.com/api/v3
```

该 Provider 调用火山方舟官方 Content Generation API：

```text
POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/{task_id}
GET  /api/v3/models
```

平台下游仍使用统一的 OpenAI 兼容视频接口。`prompt` 会转换为 Ark 的文本 `content`；`seconds` 会转换为官方 `duration`。平台只向客户返回平台视频任务 ID，不暴露 Ark 任务 ID。Ark 专用字段会先按实际上游模型版本校验，再转发；不支持的字段返回明确的参数错误，不会静默交给上游失败。

### Seedance 2.0 与 2.5 的差异

两款模型不能共用一套“无限制”的参数规则：

| 能力/限制 | Seedance 2.0 系列 | Seedance 2.5 |
| --- | --- | --- |
| 最大视频时长 | 4–15 秒，或 `-1` 自动选择 | 4–30 秒，或 `-1` 自动选择 |
| 分辨率 | 480p、720p、1080p、4K | 480p、720p、1080p |
| 参考图片 | 最多 9 张 | 最多 30 张 |
| 参考视频 | 最多 3 个 | 最多 10 个 |
| 参考音频 | 最多 3 个，不能只传音频 | 最多 10 个，可只传音频 |
| `output_format` | 不支持显式设置，使用默认 MP4 | 支持 `mp4` 和 `mov` |
| `tools` | 支持 `web_search` | 支持 `web_search` |
| `generate_audio` | 支持有声/无声视频 | 支持有声/无声视频 |
| `omni_reference_task_type` | 不支持 | 支持 `auto`、`reference`、`edit`、`extend` |
| `frames`、`seed`、`camera_fixed`、`draft` | 不支持 | 不支持 |

参考素材的 `role` 必须与类型匹配：视频使用 `reference_video`，音频使用 `reference_audio`；图片可以使用 `reference_image`、`first_frame` 或 `last_frame`。首帧/首尾帧任务与 omni reference-to-video 任务不能混用；没有显式 `role` 的单张图片按首帧图片处理。

2.0 的 `duration=-1` 表示在 4–15 秒范围内自动选择时长，编辑请求也可以使用该值。2.5 的 `duration=-1` 表示自动选择时长；视频编辑只允许 `-1`，视频扩展可以使用 `-1` 或指定 4–30 秒。2.5 的编辑、扩展以及首帧/首尾帧任务必须使用 `ratio=adaptive`；编辑和扩展必须至少提供一个 `role=reference_video` 的参考视频。如果不显式指定 `omni_reference_task_type`，平台会保留 `auto` 语义，让 Ark 根据输入素材和提示词判断任务类型。

两款模型都会把 `execution_expires_after` 限制在 3600–259200 秒，`priority` 限制在 0–9；`service_tier=flex` 不适用于 2.0/2.5。平台会在转发前检查这些约束。参考素材的具体格式、大小和内容安全限制仍以 Ark 当前官方文档及账号权限为准。

Seedance 是视频异步 Provider，不提供聊天、Embedding、图片或音频能力。渠道模型仍必须加入分组并绑定到客户 API Token，才能被下游调用。示例：

```bash
curl https://gateway.example.com/v1/videos \
  -H "Authorization: Bearer <customer-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.5",
    "prompt": "A cinematic sunrise over a quiet ocean",
    "seconds": 5
  }'
```

创建返回平台任务后，使用返回的 `id` 轮询：

```bash
curl https://gateway.example.com/v1/videos/<platform-video-id> \
  -H "Authorization: Bearer <customer-token>"

curl https://gateway.example.com/v1/videos/<platform-video-id>/content \
  -H "Authorization: Bearer <customer-token>" \
  --output seedance.mp4
```

自动获取模型会尝试 Ark 官方 `/models` 接口；如果上游账号或区域没有开放该资源，管理员应在弹窗中手动新增以上模型映射，系统不会把手动填写伪装成自动发现。

Seedance 完成任务后的官方 `usage.completion_tokens` 被记录为 `output_video_tokens`，并优先用于最终结算。2.0 系列存在官方最低 Token 用量规则，平台保留 Ark 返回的实际值，不按本地视频秒数重新推导。创建请求中的视频时长只用于预占估算；如果查询结果没有可核验的官方用量，付费任务会进入 `settlement_pending`，不会按 0 元或仅按时长错误结算。官方结果视频 URL 可能有有效期，客户应在任务完成后及时下载；2.5 的结果下载还受官方下载次数限制，平台当前不承诺无限次重复拉取同一个签名 URL。

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
