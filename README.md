# AI Token Gateway

这是一个面向多租户 AI API 网关的可部署实现，包含管理端、租户控制台、上游渠道调度、价格与额度账务以及下游兼容接口。生产发布必须满足本文的安全配置、基础设施和运维要求。

- 管理端、租户控制台和 Relay API 的 audience 隔离
- 默认拒绝的认证与权限中间件
- 租户路径范围检查
- Session Cookie 和 API Token 的数据库签发、解析与撤销接口
- 管理员安全设置（MFA 开关）、TOTP、加密 SecretBox 和追加式审计写入器
- 管理员隐藏入口：生产环境通过 `ADMIN_ENTRY_PATH=/admin-随机后缀` 隔离管理员登录页，后台登录 API 还要求同路径签发的短时 HttpOnly 通行 Cookie
- PBKDF2-SHA256 密码哈希，禁止明文密码落库
- PostgreSQL 迁移和启动 wiring
- OpenAI、Anthropic、Grok、Gemini 官方渠道的同步/流式文本对话转发，以及火山方舟 Ark Seedance 官方视频任务接口
- OpenAI Chat Completions、Responses、Models、Embeddings、Images、Audio、Videos 与 Anthropic Messages 兼容入口
- OpenAI 的官方图片、音频、视频入口；Grok Imagine/Voice 的官方图片、视频、TTS、STT 入口；Gemini 的 Imagen、原生音频和 Veo 异步任务入口；字节跳动/火山方舟 Seedance 2.0/2.5 Content Generation 异步视频入口
- 工具调用、结构化输出、图片内容块和渠道级协议转换
- Token 级 RPM、TPM、并发限制；分组 RPM、优先级、权重与故障自动降级
- 租户个人资料、邮箱/密码自助修改、SMTP 密码重置与会话失效保护
- 租户成员管理、项目 CRUD、项目成员授权和项目级角色边界
- 平台角色与权限管理：平台所有者可创建、编辑、停用角色，分配系统权限并绑定已注册管理员
- 生产公开注册的邮箱验证、一次性验证链接和验证邮件重发（SMTP）
- 基于 `github.com/pquerna/otp` 的标准 TOTP 绑定、二维码确认、启用登录校验和安全关闭
- 管理员系统设置：管理员资料、邮箱/密码、个人 TOTP、全站管理员 TOTP 策略与网站品牌配置
- 管理员全局 TOTP 强制策略开启时，高风险操作使用实时 Step-up：改价、充值、渠道写入、用户状态和关键配置变更
- 不预置任何管理员账号或开发凭据
- 真实的运营快照、使用记录、财务报表与追加式管理员审计日志；计量缺失时进入 `settlement_pending`
- 渠道出站 SSRF 防护：禁止本机、回环、私网、链路本地和云元数据地址，并在连接时复核 DNS
- CSRF 来源检查、安全响应头和显式 CORS 来源配置

## 启动

需要 Go 1.26.6 或更高版本：

```powershell
.\scripts\start-dev.ps1
```

默认监听 `:8080`，可以通过 `HTTP_ADDR` 修改。

```powershell
.\scripts\start-dev.ps1 -HttpAddr ":8090"
```

该脚本会加载未提交的 `.env.local` 后再启动服务。本地直接运行
`go run ./cmd/server` 时，必须先把 `.env.local` 中的配置导入当前
PowerShell 进程；否则数据库、认证和业务服务不会初始化。

前端在 `web/` 目录下，基于 `React + Tailwind + shadcn/ui`：

```powershell
cd web
npm install
npm run dev
```

开发时 Vite 默认监听 `:5173`，并把 API 请求代理到本机 Go 后端。生产或本地打包后，运行 `npm run build`，`go run ./cmd/server` 会优先托管 `web/dist`。

首次部署管理员：

```powershell
$env:DATABASE_URL = "postgres://..."
$env:ADMIN_EMAIL = "admin@example.com"
$env:ADMIN_PASSWORD = "use-a-long-unique-password"
$env:MFA_ENCRYPTION_KEY = "<64 hex characters>"
go run ./cmd/bootstrap-admin
```

如果没有设置 `ADMIN_MFA_SECRET`，工具会生成一个 TOTP 密钥并在命令行显示一次。请立即将它导入认证器并妥善保存。管理员后台里的 MFA 安全开关默认关闭，登录后可在安全设置中显式开启。该工具不会创建默认账号，重复使用相同邮箱会失败。

渠道需要管理员在后台手动新增，填写 Base URL、API Key 和模型映射。API Key 会加密保存到 `channel_secrets`，渠道列表只返回脱敏预览。
后台渠道管理支持弹窗新增、编辑、暂停、启用和软删除；填写上游 Base URL 与 API Key 后，可以通过 models 接口自动读取模型，也可以手动维护模型映射。软删除会同时吊销该渠道的活跃密钥，模型映射关闭“下游可见”后不会参与 Relay 路由。

## 当前接口

```text
GET  /healthz
GET  /admin/v1/me
GET  /console/v1/profile
PUT  /console/v1/profile
PUT  /console/v1/profile/email
PUT  /console/v1/profile/password
GET  /console/v1/profile/mfa
POST /console/v1/profile/mfa/enroll
POST /console/v1/profile/mfa/enroll/{enrollmentID}/confirm
POST /console/v1/profile/mfa/disable
POST /console/v1/auth/email/verify
POST /console/v1/auth/email/resend
GET  /console/v1/tenants/{tenantID}/members
POST /console/v1/tenants/{tenantID}/members
PUT  /console/v1/tenants/{tenantID}/members/{userID}
DELETE /console/v1/tenants/{tenantID}/members/{userID}
GET  /console/v1/tenants/{tenantID}/projects
POST /console/v1/tenants/{tenantID}/projects
PUT  /console/v1/tenants/{tenantID}/projects/{projectID}
DELETE /console/v1/tenants/{tenantID}/projects/{projectID}
GET  /console/v1/tenants/{tenantID}/projects/{projectID}/members
POST /console/v1/tenants/{tenantID}/projects/{projectID}/members
PUT  /console/v1/tenants/{tenantID}/projects/{projectID}/members/{userID}
DELETE /console/v1/tenants/{tenantID}/projects/{projectID}/members/{userID}
GET  /admin/v1/profile
PUT  /admin/v1/profile
PUT  /admin/v1/profile/email
PUT  /admin/v1/profile/password
GET  /admin/v1/auth/mfa/status
POST /admin/v1/auth/mfa/disable
GET  /admin/v1/settings
PUT  /admin/v1/settings
PUT  /admin/v1/settings/site
GET  /admin/v1/settings/email
PUT  /admin/v1/settings/email
POST /admin/v1/settings/email/test-connection
POST /admin/v1/settings/email/test-message
GET  /admin/v1/settings/email/templates
POST /admin/v1/settings/email/templates
PUT  /admin/v1/settings/email/templates/{templateID}
DELETE /admin/v1/settings/email/templates/{templateID}
GET  /admin/v1/settings/features
PUT  /admin/v1/settings/features
GET  /public/v1/settings
GET  /public/v1/features
GET  /admin/v1/channels
POST /admin/v1/channels/discover-models
POST /admin/v1/channels
PUT  /admin/v1/channels/{channelID}
POST /admin/v1/channels/{channelID}/pause
POST /admin/v1/channels/{channelID}/enable
DELETE /admin/v1/channels/{channelID}
GET  /admin/v1/prices
POST /admin/v1/prices/publish
GET  /admin/v1/usage
GET  /admin/v1/finance
GET  /admin/v1/overview
GET  /admin/v1/ops
GET  /admin/v1/model-status
GET  /admin/v1/audit
GET  /admin/v1/tenants/{tenantID}/billing/account
POST /admin/v1/tenants/{tenantID}/billing/credit
POST /admin/v1/usage/{requestID}/settle
GET  /admin/v1/roles
GET  /admin/v1/permissions
POST /admin/v1/roles
PUT  /admin/v1/roles/{roleID}
POST /admin/v1/roles/{roleID}/disable
GET  /admin/v1/users/{userID}/roles
PUT  /admin/v1/users/{userID}/roles
GET  /console/v1/tenants/{tenantID}/usage
GET  /console/v1/tenants/{tenantID}/model-status
GET  /console/v1/tenants/{tenantID}/billing/account
POST /v1/chat/completions
POST /v1/embeddings
GET  /v1/models
POST /v1/responses
POST /v1/messages
POST /v1/images/generations
POST /v1/images/edits
POST /v1/audio/transcriptions
POST /v1/audio/translations
POST /v1/audio/speech
POST /v1/videos
GET  /v1/videos/{videoID}
GET  /v1/videos/{videoID}/content
```

除 `/healthz`、公开模型目录、公开系统设置和认证入口外，接口默认需要管理员 Session、租户 Session 或 Relay API Token。下游接口支持同步/流式文本、工具调用、结构化输出、图片内容块、Embeddings、图片生成/编辑、音频转写/翻译/语音生成和视频异步任务；`GET /v1/models` 会按 Token 的模型白名单过滤。视频任务返回平台任务 ID，调用方通过平台接口查询和下载结果。

对外 API 终端由管理员在系统设置中配置为网关根地址。客户可使用 `<gateway-root>/v1` 对接 OpenAI 兼容客户端，或使用 `<gateway-root>` 对接 Anthropic / Claude 客户端。服务端同时兼容根路径、标准 `/v1` 和客户端误拼接的 `/v1/v1`，但推荐始终从控制台 API 令牌页面复制地址。

管理员“系统设置”中的邮件设置、模板和功能开关独立于网站基础设置。邮件总开关默认关闭；关闭时注册、密码重置和通知流程不会强制依赖 SMTP，开启后再按事件开关和模板配置发送。

租户控制台的“模型状态”页面通过 `GET /console/v1/tenants/{tenantID}/model-status` 按分组展示模型的可用路由数、自动熔断状态、连续失败次数以及最近成功/失败时间。活动分组可见，租户已有令牌绑定的停用分组也会显示；该接口只读取分组映射和渠道运行状态，不主动请求上游；前端进入页面后每 15 秒刷新一次。新建但尚未产生真实请求的路由显示为“待观测”，不能误认为已经完成上游探针验证。

管理员系统设置的“功能开关”可以单独启用或关闭模型状态功能。关闭后，公开配置 `GET /public/v1/features` 返回 `model_status_enabled: false`，租户菜单、深链和 `GET /console/v1/tenants/{tenantID}/model-status` 同时关闭；管理员“模型监控”页面仍用于平台运营查看全部分组和模型，不受租户入口开关影响。管理员监控通过 `GET /admin/v1/model-status` 展示分组模型、路由可用数、最近真实请求、最近延迟、近 7 天可用率和状态条，不执行主动上游探针。

管理员 Token 列表仅用于平台运营查看脱敏记录、调整分组和撤销；`POST /admin/v1/tokens` 和 `POST /admin/v1/users` 保留为明确拒绝的兼容入口，均返回 `403`。用户账号和用户 API Token 必须分别通过公开注册和租户控制台自行创建。

## 额度与计费

请求进入 Relay 后，系统会在 PostgreSQL 事务中完成：

1. 按 `Token -> Project -> Tenant -> Platform default` 查找当前生效的价格版本。
2. 根据输入/输出 token 估算并原子预占租户预付余额。
3. 上游成功后按实际 Usage 结算，释放预占差额并写入用量事件和双重记账流水。若最终费用超过预占，账户会完整记录实际费用并可能暂时为负数，直到充值前不会再通过预占。
4. 上游失败且未向下游发送流式数据时释放预占；已经产生输出但缺少可靠 Usage 时进入 `settlement_pending` 等待对账。故障切换复用同一笔预占，最终账单跟随成功渠道。

价格使用 `price_versions` 保存，发布新价格会创建新版本，不会修改历史价格。官方参考价保存于 `official_model_price_versions`，可从 LiteLLM 数据源同步；模型广场显示 Token 组件的每 1M Token 参考价、媒体/请求/查询/会话/页数/秒数等非 Token 组件价格，以及实际配置分组的价格乘倍率结果。价格版本支持输入、输出、缓存创建/读取、推理、音频/图片/视频、图片数量、像素、字符、秒数、请求、查询、会话、页数、搜索、Grounding、OCR、存储、代码解释器、DBU、Priority/Flex/Batch 和阶梯价格。LiteLLM 的 `tiered_pricing` 与 `*_above_N_tokens` 按本次上下文选择整单价格，`*_interval` 才按数量分段。缺少实际计量价格组件时，付费请求不会按零价格放行。

管理员可以在后台的“额度计费”页面发布平台默认价格，并使用租户 ID 给预付账户入账。充值接口必须带幂等键；Relay 请求也支持 `Idempotency-Key` 请求头。缺少价格或预付账户时，Relay 会拒绝请求，不会在未配置计费规则的情况下放行。

## 数据库迁移

迁移位于：

```text
migrations/
```

应用启动时会在配置 `DATABASE_URL` 后自动执行迁移。生产环境不要使用数据库超级用户连接应用。

## 安全与运行约定

- 上游 API Key 只保存为 AES-GCM 加密 Secret，列表接口只返回脱敏预览；不要把 `API_KEY`、Session、Token 或密码写入日志。
- API Token 只在创建响应中返回完整值，之后只能看到前缀。Token 可绑定项目、模型、分组、IP/CIDR、域名、过期时间和 RPM/TPM/并发限制。
- API Token 只能由租户用户在控制台创建和撤销；管理员不能代发用户密钥。创建者账号被锁定、停用或移出租户时，相关 Token 立即失效并在账号停用时标记为 revoked。网络白名单支持 IP/CIDR 或浏览器域名来源策略；服务端建议使用 IP/CIDR，域名策略依赖可伪造的 Origin/Referer。
- TOTP 功能主开关默认关闭。开启后，租户用户和管理员都可在各自个人资料中自主绑定或解除 TOTP；个人绑定只保护该账号，不会自动强制其他账号使用。
- 管理员全局 MFA 默认关闭。开启前，所有 active 平台管理员都必须先完成个人 TOTP 绑定；全局策略开启后，管理员登录及改价、充值、渠道新增/编辑/暂停/启用/删除、Token 运营变更、用户状态和系统设置写入才要求 `X-MFA-Code` 实时 Step-up。管理员个人资料、密码、邮箱和 MFA 修改会使旧 Session 失效。
- 对于上游成功但没有可靠 Usage 的 `settlement_pending` 请求，管理员可在使用记录中核对真实计量后调用补结算接口；接口拒绝空计量，避免误记为 0 元。
- 生产必须使用 HTTPS、`COOKIE_SECURE=true` 和明确的 `CORS_ALLOWED_ORIGINS`。SMTP 地址、发件人、用户名、密码、公开访问地址和邮件模板由管理员在“系统设置 → 邮件设置”配置，敏感密码使用 AES-GCM 保存；标准部署不需要在环境文件填写 SMTP。
- 生产必须显式设置 `REGISTRATION_ENABLED`，它只在首次连接数据库时写入“开放用户注册”的初始状态，之后不会覆盖管理员后台的选择。公开注册由“系统设置 → 功能开关 → 开放用户注册”实时控制；邮箱验证则由邮件总开关和“邮箱验证码”开关共同控制。注册接口另有 IP/邮箱节流。Captcha、Bot 防护和 WAF 仍应在 Caddy 前的 CDN/WAF 边缘层配置，不能由应用内置节流替代。
- 平台角色只能由 `platform_owner` 通过角色管理页面和对应 API 维护；角色权限不是管理员绑定权限，服务层会再次校验平台所有者身份，并保护最后一个有效平台管理员。
- 生产在 Nginx 等反向代理后运行时，必须配置 `TRUSTED_PROXY_CIDRS`，仅信任这些代理追加的 `X-Forwarded-For`；这决定 Token IP 白名单和使用记录中的客户端 IP。
- 生产应用进程必须监听回环地址（例如 `127.0.0.1:8080`），并按流式与媒体请求配置 `HTTP_READ_TIMEOUT`、`HTTP_WRITE_TIMEOUT`、`HTTP_IDLE_TIMEOUT`；默认写超时为 15 分钟。
- 审计日志只追加不提供业务删除接口；IP 和 User-Agent 在审计中保存哈希，使用记录中的客户端 IP 用于管理员排障与账务追踪。
- `migrations/` 中的迁移按文件名顺序执行，当前发布目录最高版本为 `041_official_price_runtime_billing.sql`；所有环境都应通过应用启动自动执行，发布时必须让二进制、前端和迁移来自同一提交。

## 重要限制

当前代码已经覆盖核心商用闭环，但以下能力仍需要单独建设后才能对外承诺：

- WebAuthn 支持
- 短信通知和多通道账号恢复
- 主动渠道健康探针、跨实例错误聚合和通知告警
- Anthropic 专用 Embeddings，以及具体上游账户未开放的媒体模型
- Responses 的完整高级事件语义和部分服务端工具
- 套餐、退款、对账、自动充值和财务/使用记录导出
- Redis 或其他共享存储的分布式限流，以及多实例并发协调

接口接入示例见 `docs/08-api-integration.md`；数据表、权限和安全基线的详细说明见 `docs/01-role-permission-matrix.md` 至 `docs/08-api-integration.md`。Linux 标准部署、生产配置、TLS、SMTP、备份、更新和回滚流程见 `docs/10-linux-caddy-pm2-deployment.md`。旧版 Nginx + systemd 文档已停用，不应与当前流程混用。
