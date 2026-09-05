# 当前能力与交付边界

本文档记录当前代码实际提供的行为，优先级高于 UI 文案和早期设计草稿。

## 身份边界

| 区域 | 认证 | 访问范围 |
| --- | --- | --- |
| `/admin/*` | `admin_session` | 平台角色和平台权限 |
| `/console/*` | `console_session` | 当前租户、成员角色和项目集合 |
| `/v1/*` | `Authorization: Bearer`、`X-API-Key` 或 Anthropic `x-api-key` | API Token 的租户、项目、模型、网络和速率策略 |

管理员 Session 不能作为下游 Token 使用，租户 Session 不能访问平台管理接口。控制台 Token 列表以及编辑、暂停、启用、终止、删除操作都按 `tenant_id + created_by` 查询，只能操作本人创建的 Token；管理员列表用于平台运营，能看到全平台脱敏摘要。完整密钥只在创建结果中产生，并按用户和租户保存在当前浏览器，刷新后仍可复制；清除浏览器存储后服务端无法从哈希恢复。平台所有者可以在独立角色页面维护平台角色、权限集合和已注册管理员绑定，普通平台角色不能越权修改这些绑定。

生产公开注册会创建 `pending` 用户，SMTP 发送一次性验证链接；只有 `POST /console/v1/auth/email/verify` 成功后才能登录，后台状态修改也不能绕过邮箱验证。开发环境默认关闭该验证开关以便本机测试。验证码/Captcha 和 WAF 仍由边缘基础设施提供。

用户可以在控制台提交企业认证资料。企业名称、统一社会信用代码、营业执照和对公账户用于企业级发票等能力；营业执照与银行账号加密保存。用户只能查看自己租户的认证状态和脱敏银行账号，管理员可在企业认证页面按待审核、已通过、已拒绝筛选，查看详情、下载营业执照并审核。拒绝必须填写原因，拒绝申请可重新提交，已通过申请不能重复提交。

账单与额度页面提供在线充值。系统支持独立启用的微信支付 Native、支付宝当面付、Stripe Checkout 和 PayPal Orders/Capture。支付配置位于管理员“系统设置 -> 支付配置”，充值套餐位于同级的“套餐配置”，所有支付方式共用全局充值金额、到账金额、赠送金额、活动时间和有效期（0 表示长期有效）；敏感配置加密保存。支付订单由平台生成幂等商户单号，只有官方签名回调或官方 API 验证结果才会写入租户账务。Stripe 的 Webhook 地址为当前域名下的 `/payments/webhooks/stripe`，成功/取消返回地址由 Checkout Session 自动生成并回到充值页。Stripe 支付方式可显式勾选卡、Alipay、WeChat Pay，或交给 Stripe 动态选择；Apple Pay/Google Pay 随卡支付钱包资格展示。不同国家的固定白名单规则目前不在平台配置中。

## 路由与兜底

请求先校验 Token、网络白名单、模型白名单、Token 限流和分组 RPM，再从 `models -> channel_models -> channels` 找候选渠道。候选渠道按 priority 从高到低分层；同一优先级按 weight 加权选择。上游返回可重试错误、超时或凭据不可用时，继续尝试其他候选渠道。连续失败达到阈值后，渠道自动进入短时熔断窗口；成功后清零。

管理员可以为渠道配置独立的上游账户查询系统和凭据，用于查看第三方中转平台的余额与倍率。服务启动后立即执行一轮账户信息同步，之后每 1 分钟自动执行；管理员也可以通过渠道页面手动立即同步。自动任务只处理已配置查询凭据且对接系统为 New API 或 Sub2API 的渠道，单个渠道的失败不会阻塞其他渠道。该查询是旁路运营信息：不参与候选渠道筛选、调度排序、健康记录、熔断、客户计费或上游成本估算；未配置、查询失败或第三方没有对应接口时，渠道仍按原有配置正常调用。

分组是渠道集合和计费/资源策略，不是模型本身。一个模型可以由多个分组提供，一个分组也可以拥有多个模型。Token 绑定一个分组后，只能使用该分组实际关联渠道提供的模型。

租户控制台提供 `GET /console/v1/tenants/{tenantID}/model-status`。它只返回管理员创建且启用的模型监控配置：管理员可在监控范围内手动指定主模型，租户端按分组呈现一张主模型健康卡，其他模型作为备用模型展开查看。监控选择“全部模型”时动态聚合该分组当前启用的渠道模型映射，选择“指定模型”时保留管理员选择的模型，即使当前没有可用渠道也显示为 `unavailable`。主模型必须属于当前监控范围；未指定时服务端回退为范围内排序最前的模型。活动分组可见，租户已有令牌绑定的停用分组也可见；未配置或已停用监控的分组不会返回。分组停用显示 `disabled`，没有可用路由显示 `unavailable`，部分路由因渠道或模型映射停用、自动熔断或主动探测失败显示 `degraded`，所有路由可用且全部路由有真实请求或主动探测观测显示 `normal`；尚未完成观测显示 `pending`。页面按监控配置显示最近 30、60 或 120 次健康请求状态，默认 60；同时展示最近延迟和近 7 天健康度。主动探测请求写入系统健康记录，不产生客户账单，但会参与当前路由健康判断，前端默认每 15 秒轮询。

模型状态是可选功能，管理员可在系统设置的“功能开关”中关闭。关闭后租户侧入口、深链和接口均不可用，返回 `MODEL_STATUS_DISABLED`；公开 `GET /public/v1/features` 只返回非敏感的功能标志。管理员运营侧的“模型监控”先按分组创建配置，可选择全部模型或指定一个或多个模型，并从当前范围手动设置主模型，设置被展示的最近请求数（30、60、120，默认 60），并可启用按自定义周期运行的主动探测。管理员通过 `GET /admin/v1/model-status` 查看已启用监控覆盖的分组和模型，按分组显示路由可用数、渠道模型熔断状态、主动探测健康请求、最近响应延迟、近 7 天可用率和最近请求状态。主动探测使用单字符、1 Token 的最小请求，保存后立即执行并覆盖监控范围内全部模型和候选渠道；探测请求写入系统健康记录，但不计入客户账单或客户使用记录。最多并发 4 个分组，同一配置不会重叠执行，完成后按配置间隔并加入少量随机抖动安排下一次。近 7 天可用率统计统一健康请求中的 `settled`、`settlement_pending` 和 `failed` 状态，其中 `settlement_pending` 表示上游已接受但用量仍待对账。

## 计费与记录

付费请求先在 PostgreSQL 事务中预占余额，成功后用上游 Usage 结算并释放差额，未向下游发送任何流式数据的上游失败会释放预占；已经产生输出但无法安全取得完整 Usage 的请求会保留预占等待对账，避免把上游已产生的费用当成免费。一次下游请求在故障切换期间只有一笔预占和一笔最终账单。免费分组也写入 `model_requests`，费用为零。`Idempotency-Key` 防止相同请求重复预占或结算。

记录包含 Token 前缀、租户、模型、厂商、分组、端点、客户端 IP、请求类型、服务等级、输入/输出/缓存/推理 Token、图片/音频/视频 Token、请求/查询/会话/页数/秒数等实际计量、逐组件费用、价格快照、费用、延迟、状态和时间。管理员可用租户、模型、分组、状态、搜索词和时间范围查询使用记录；财务报表的余额、消耗、充值和交易列表也支持时间范围。

## 价格

`official_model_price_versions` 保存 LiteLLM 同步的官方参考价；`price_versions` 保存平台手动发布的当前价格版本。运行时客户扣费优先匹配 Token、项目、租户和平台手动价格；没有手动价格时，已同步的 LiteLLM 官方价格会直接作为客户计费基准。管理员上游成本独立优先使用请求发生时的官方参考价，再乘渠道的 `upstream_cost_discount`；官方价未同步时才回退到客户价格，并将成本依据写入 `upstream_price_snapshot_json`。模型广场只展示已存在有效渠道映射的模型，分组调用价只展示实际关联该模型渠道的 active 分组，并以：

```text
平台调用价 = 官方参考价 × 分组倍率
```

Token 价格按 USD / 1M Token 展示；非 Token 组件按自身单位展示，例如 USD / image、USD / second、USD / request、USD / 1K calls。LiteLLM 是统一的参考源，不保证实时或覆盖所有官方价格；管理员可以发布平台价格版本。平台调用价为对应价格组件乘分组倍率。价格缺失时付费 Relay 请求拒绝，不按零价格放行。

管理员财务页的当前价格矩阵同时展示每个模型在各个有效分组中的平台调用价、预估上游成本、预估利润和利润率。平台调用价使用官方/平台参考价乘分组倍率；预估成本使用官方参考价乘渠道成本折扣，并取该分组当前优先级最高的可用渠道作为展示基准。备用渠道的实际成本以请求最终选中的渠道及其折扣为准；免费分组没有客户收入，利润率显示为不适用。

## 下游兼容范围

当前开放：

- `POST /v1/chat/completions`，支持同步和 SSE 流式响应；OpenAI/Grok 渠道保留工具、函数、结构化输出和内容块字段
- `GET /v1/models`
- `POST /v1/embeddings`，支持 OpenAI/Grok OpenAI-compatible Embeddings 和 Gemini EmbedContent
- `POST /v1/responses` 的文本/内容块基础映射与流式输出
- `POST /v1/messages` 的 Anthropic 文本、图片内容块、工具定义/工具结果与流式输出
- `POST /v1/images/generations`、`POST /v1/images/edits`
- `POST /v1/audio/transcriptions`、`POST /v1/audio/translations`、`POST /v1/audio/speech`
- `POST /v1/videos`、`GET /v1/videos/{videoID}`、`GET /v1/videos/{videoID}/content`

图片输入、工具调用和结构化输出会按渠道协议转换；具体模型是否支持仍以渠道和上游模型能力为准。OpenAI 使用官方 OpenAI API；Grok 的文本/Embedding 使用 OpenAI-compatible API，媒体使用 xAI 官方路径；Gemini 使用官方 GenerateContent、EmbedContent、Imagen 和 Veo 协议；Anthropic 仅开放其官方 Messages 能力，不伪装成图片/视频生成或 Embedding 渠道；字节跳动/火山方舟使用官方 Ark Content Generation API，支持 Seedance 2.0/2.5 视频异步任务。Seedance 2.0 与 2.5 使用独立的模型能力描述和请求校验：2.0 的视频时长上限为 15 秒并支持 4K，2.5 的视频时长上限为 30 秒并支持 MOV、更多参考素材及单独音频输入。视频为异步任务，平台保存任务与账务关联并只向客户返回平台任务 ID。客户端提交不适用能力会得到稳定的 unsupported/invalid 错误，不会静默降级成文本请求。

流式和非流式请求在上游明确提供 Usage 时按真实 Usage 结算；OpenAI 的 `prompt_tokens`/`completion_tokens`、Gemini 的 prompt/candidates/thoughts、Anthropic 的 input/output/cache creation/cache read 都按父量与子集关系归一化，缓存、推理和媒体 Token 不会重复计费。媒体优先使用上游 Usage，其次使用能够从请求或文件元数据验证的图片数量、音频时长、视频秒数和语音字符数；无法可靠推导的计量不会伪造为 0，付费请求会返回明确的 Usage 不可用错误。

支持的官方参考价格组件包括普通/缓存创建/缓存读取/推理/音频/图片/视频 Token，图片数量、像素、字符、秒数、请求、查询、会话、页数、文件检索、向量存储、Grounding、搜索、OCR、代码解释器、DBU，以及 Priority、Flex、Batch 和上下文阶梯价格。LiteLLM 的 `tiered_pricing` 与 `*_above_N_tokens` 会按官方规则选择整单上下文档位；带 `*_interval` 的计量才按该组件数量做分段。媒体生成和音频接口已经有路由，但仍受渠道模型映射和上游账户权限约束。火山方舟渠道只实现视频任务创建、查询和结果下载；不会把不支持的聊天、Embedding、图片或音频请求转发到 Ark。

## 安全执行点

- 渠道 URL 保存和模型发现都会校验协议、Host、无凭据、无 query/fragment，并禁止 loopback、private、link-local、multicast、unspecified 和 metadata 地址。
- 实际 HTTP 连接重新解析 DNS 并只拨号到校验过的公网地址，禁止自动跟随重定向。
- Session 写操作执行来源校验；跨域调用只允许 `CORS_ALLOWED_ORIGINS` 中的明确来源。
- 管理员和控制台密码、邮箱、MFA 修改后撤销该用户旧 Session。
- 管理端 mutation、认证 mutation 和 Relay 请求写入追加式审计日志；请求体、Authorization、API Key、上游密钥不会写入审计。
- `POST /admin/v1/usage/{requestID}/settle` 允许具备 `billing:update` 的管理员为 `settlement_pending` 请求补录真实 Usage；空计量会被拒绝，补结算操作会进入审计日志。
- Token 网络白名单支持 IP/CIDR 或浏览器域名来源策略；IP/CIDR 匹配真实对端地址，域名匹配 Origin/Referer。服务端调用应使用 IP/CIDR，域名请求缺少浏览器来源头时会被拒绝；非浏览器客户端可以伪造来源头，因此域名不应作为强 bearer-token 边界。
- 管理员全局 MFA 策略开启时，角色/权限写入和管理员绑定必须携带实时 `X-MFA-Code`；连续错误会返回 `MFA_STEP_UP_THROTTLED`。`platform_owner` 角色定义不可编辑或停用，最后一个有效平台管理员不能被移除；角色权限/状态改变会立即撤销受影响管理员的后台 Session。普通 `user:update` 管理员不能修改平台所有者账号。

## 发布前检查

1. 设置独立的非超级用户 PostgreSQL 账号、随机 `TOKEN_PEPPER`、`SESSION_PEPPER` 和 `MFA_ENCRYPTION_KEY`。
2. 生产设置 `COOKIE_SECURE=true`，通过 TLS 反向代理部署，并配置明确的 `CORS_ALLOWED_ORIGINS`。
3. 运行 `go test ./...`、`go vet ./...` 和 `cd web; npm run build`。
4. 用真实管理员完成 TOTP 绑定后再开启全局管理员 MFA，并为所有管理员准备恢复流程。
5. 开启公开注册前，在“系统设置 → 邮件设置”配置 HTTPS 公开地址、SMTP STARTTLS 和邮箱验证，并在边缘层启用 Captcha/WAF。
6. 验证 Token 白名单、过期、撤销、RPM/TPM/并发限制、分组兜底和账务幂等。
7. 对 Anthropic 专用 Embeddings、完整 Responses 高级事件和其他未实现能力不要在客户文档或首页承诺为已支持功能。
