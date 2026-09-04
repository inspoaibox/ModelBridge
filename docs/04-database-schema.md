# 核心数据库设计

## 1. 基础约定

首版建议使用 PostgreSQL。数据库设计遵循以下约定：

- 主键使用应用生成的 UUIDv7 或等价的不可猜测 ID。
- 时间统一使用 UTC，并保存 `created_at`、`updated_at`。
- 金额使用整数最小货币单位或 `DECIMAL`，禁止使用浮点数。
- 所有租户业务表必须带 `tenant_id`。
- 删除用户、Token、渠道和账务数据优先使用软删除或状态变更。
- 业务写操作使用事务；跨服务操作通过 Outbox 事件保证最终投递。

## 2. 身份与组织

### `users`

```text
id
email
phone
password_hash
status                 active | locked | disabled | pending
email_verified_at
last_login_at
password_changed_at
created_at
updated_at
deleted_at
```

约束：

- `email` 规范化后唯一。
- `password_hash` 只能保存密码哈希，不能保存明文或可逆密码。
- 禁止通过普通用户接口修改 `status`。

### `platform_roles`、`platform_permissions`

```text
platform_roles(id, code, name, status)
platform_permissions(id, resource, action, name)
platform_role_permissions(role_id, permission_id)
platform_user_roles(user_id, role_id)
```

平台角色由后台角色管理 API 维护；角色权限关系使用 `platform_role_permissions`，管理员绑定使用 `platform_user_roles`。公开注册只写入租户成员关系，不写入平台角色关系。

平台角色与租户角色使用不同表或至少不同的作用域字段，避免角色混用。

### `tenants`

```text
id
name
slug
status                 active | suspended | closed
currency
settings_json
created_at
updated_at
deleted_at
```

### `tenant_members`

```text
id
tenant_id
user_id
role_code              tenant_owner | tenant_admin | developer | viewer
status                 invited | active | suspended | removed
created_by
created_at
updated_at
```

约束：

- `(tenant_id, user_id)` 唯一。
- `tenant_owner` 数量和转移规则由服务层明确控制。
- 成员被移除后，相关 Session 和项目授权立即失效。

### `projects`

```text
id
tenant_id
name
slug
status
created_by
created_at
updated_at
deleted_at
```

### `project_members`

```text
project_id
user_id
role_code              project_admin | developer | viewer
created_at
```

## 3. Session、MFA 与 API Token

### `web_sessions`

```text
id
user_id
audience                admin | console
session_hash
auth_strength           password | password_mfa | passkey
ip_hash
user_agent_hash
expires_at
last_seen_at
revoked_at
created_at
```

只保存 Session 摘要。登录、提权、密码变更、MFA 变更后应使旧 Session 失效或重新验证。

### `mfa_credentials`

```text
id
user_id
type                    webauthn | totp
credential_public_key
encrypted_secret
last_used_at
created_at
revoked_at
```

恢复码只保存单向摘要，并且每个恢复码只能使用一次。

### `api_tokens`

```text
id
tenant_id
project_id
created_by
name
token_prefix
token_hash
scopes_json
allowed_models_json
allowed_ips_json
allowed_domains_json
rate_limit_json
expires_at
last_used_at
status                  active | disabled | revoked | expired
created_at
revoked_at
deleted_at
```

约束：

- `token_hash` 唯一。
- 完整 Token 只在创建响应中返回一次。
- `disabled` 表示用户主动暂停，`revoked` 表示永久终止，`deleted_at` 表示用户列表软删除。
- 删除和终止只会让 Token 失效，不复用原 Token，也不删除历史用量记录。
- Token 只允许访问其所属租户和项目；解析时还要求创建者账号、租户成员状态和项目角色仍然有效。
- 创建者被锁定/停用、租户成员被暂停/移除、项目成员被降为只读或移除时，相关 Token 会失效或被撤销。

## 4. 模型、渠道与价格

### `models`

```text
id
provider
model_name
protocol_family
capabilities_json
status
created_at
updated_at
```

### `channels`

```text
id
name
provider
base_url
credential_ref
status                  active | draining | disabled
upstream_cost_discount  上游成本估算折扣；与账户查询倍率完全独立
upstream_integration    official | newapi | sub2api | other
upstream_account_credential_ref
                        账户查询凭据引用，不参与 Relay 调用
upstream_balance        管理员查看的上游余额快照，可为空
upstream_balance_unit   USD、quota 等上游返回的单位
upstream_rate_multiplier
                        上游账户/分组倍率快照，可为空
upstream_account_sync_status
                        not_configured | pending | success | failed | not_supported
upstream_account_sync_error
                        最近一次查询失败原因，不影响渠道
upstream_account_synced_at
                        最近一次成功同步时间
upstream_account_last_attempt_at
                        最近一次查询尝试时间
priority
weight
timeout_policy_json
retry_policy_json
health_status_json
created_at
updated_at
deleted_at
```

`credential_ref` 只保存密钥管理系统中的引用，不保存明文凭据。
`upstream_account_credential_ref` 与 `credential_ref` 分开，专门用于可选的余额/倍率查询；账户查询快照是运营展示数据，不得被路由、健康度、计费或成本估算读取。

### `channel_account_secrets`

```text
id
channel_id
encrypted_secret       独立账户查询凭据的密文
secret_prefix
secret_suffix
created_by
created_at
rotated_at
revoked_at
```

该表只保存账户查询凭据。渠道调用 API Key 仍保存在 `channel_secrets`，两者不可混用。

### `channel_models`

```text
channel_id
model_id
upstream_model_name
enabled
health_status
consecutive_failures
auto_disabled_until
last_failure_status
last_failure_at
last_success_at
created_at
updated_at
```

### `price_versions`

```text
id
scope_type              platform_default | tenant | project | token
scope_id                平台默认价格为 NULL；租户/项目/Token 价格为对应资源 ID
model_id
currency
input_price_per_unit
output_price_per_unit
cached_input_price_per_unit
reasoning_price_per_unit
minimum_charge
version
effective_from
effective_to
status                  draft | active | retired
created_by
created_at
```

价格发布后不可原地修改。修正价格应创建新版本。运行时优先使用 Token、项目、租户和平台手动价格；没有手动价格时使用已同步的 `official_model_price_versions` 官方参考价格。付费请求只会在 `price_version_id` 和 `official_price_version_id` 中记录一个价格版本，并将实际组件与分组倍率写入价格快照。

### `price_components` / `official_price_components`

价格版本的可计费组件明细。`price_per_unit` 对 Token 组件表示每 Token 原始单价；界面按每 1M Token 展示。非 Token 组件使用自身单位，例如 `image`、`second`、`request`、`query`、`session`、`page`、`1k_calls`。`tier_json` 保存按上下文长度、缓存时长或其他阈值变化的阶梯单价，`metadata_json` 保存来源字段和原始参考数据。

## 5. 请求、用量和计费

### `model_requests`

```text
id
request_id              对外或内部幂等请求 ID
tenant_id
project_id
token_id
model_id
channel_id
price_version_id          手动价格版本；与 official_price_version_id 二选一
official_price_version_id LiteLLM 官方价格版本；与 price_version_id 二选一
status                  started | settlement_pending | failed | settled
provider_request_id
input_tokens            上游报告的输入 token 总数
output_tokens
cached_input_tokens
reasoning_tokens
usage_metrics_json        JSON 中保存音频/图片/视频 Token、图片数量、像素、字符、秒数、请求、查询、会话、页数、搜索、Grounding、OCR、存储等完整计量
charge_breakdown_json     逐组件数量、单价和金额
price_snapshot_json       本次请求使用的价格组件、版本和分组倍率快照
upstream_price_snapshot_json
                         上游成本估算使用的官方参考价或客户价格回退快照
upstream_cost_discount    请求实际使用渠道的上游成本折扣系数快照
estimated_amount
settled_amount
estimated_upstream_cost   按请求估算用量计算的上游成本
upstream_cost             按最终用量计算的上游成本
currency
started_at
finished_at
created_at
```

`request_id` 必须唯一。请求状态只能按状态机推进，不能由客户端直接设置。

### `usage_events`

```text
id
request_id
source                   upstream | local_estimate | reconciliation
input_tokens
output_tokens
cached_input_tokens
reasoning_tokens
usage_metrics_json
charge_breakdown_json
raw_usage_json
event_version
created_at
```

同一个请求的同一种来源和版本只能成功写入一次，防止重试重复结算。

### `billing_reservations`

```text
id
request_id
tenant_id
account_id
reserved_amount
settled_amount
released_amount
currency
status                   held | pending | settled | released | expired
expires_at
created_at
updated_at
```

请求开始时创建预占；请求完成、失败或超时后都必须完成结算或释放流程。

### `media_jobs`

视频等异步媒体任务保存独立的任务状态，但通过 `model_request_id` 和 `reservation_id` 绑定到同一条请求与账务记录：

```text
id
model_request_id       唯一关联的 model_requests ID
reservation_id         可选的 billing_reservations ID
tenant_id / project_id / token_id
group_id / channel_id
provider / model_name
upstream_model_name     渠道映射后的实际上游模型名
upstream_job_id        仅服务端保存，不返回给下游
status                 queued | processing | completed | failed | cancelled
output_uri
response_json
estimated_metrics_json
failure_reason
created_at / updated_at / completed_at
```

任务查询必须同时匹配 `tenant_id + token_id`。任务创建时产生的预占一直保持到上游完成或失败；成功任务按实际用量结算，失败任务释放预占。上游成功但缺少可靠计量时进入 `pending`，由管理员补录真实用量后结算，防止异步轮询造成重复扣费、漏账或遗留余额占用。

### `email_verification_tokens`

生产公开注册时使用该表保存邮箱验证令牌摘要：

```text
id
user_id
token_hash
requested_ip_hash
expires_at
used_at
created_at
```

令牌只返回到注册邮箱，30 分钟过期且只能使用一次；应用不保存明文令牌。

### Token 限流和 Step-up 失败记录

`api_token_rate_windows`、`api_token_concurrency` 分别保存 Token 的 RPM/TPM/并发窗口。管理员 Step-up 失败复用 `login_throttles` 的独立哈希命名空间，不保存 TOTP 码；达到阈值后短时锁定。

`api_tokens.spend_limit` 是单个 Token 的客户侧累计消费上限，`0` 表示不限制；`api_tokens.spent_amount` 是已成功结算的累计客户侧费用。付费请求预留时会锁定 Token 行并合并未完成预留金额进行额度判断，结算时在同一事务中累加 `spent_amount`。失败释放的预留不会累加，额度耗尽后的新请求返回 `TOKEN_SPEND_LIMIT_REACHED`。

### `ledger_accounts`

```text
id
tenant_id
account_type             prepaid_balance | credit | receivable
currency
balance                  预付账户可用余额缓存；系统账户也可保存汇总余额
account_code             系统账户标识，例如 system:revenue:USD
is_system
status
created_at
```

### `ledger_transactions`、`ledger_lines`

```text
ledger_transactions(
  id,
  idempotency_key,
  transaction_type,
  reference_type,
  reference_id,
  created_at
)

ledger_lines(
  id,
  transaction_id,
  account_id,
  direction,
  amount,
  currency,
  metadata_json,
  created_at
)
```

账务流水只允许追加。退款不是修改原消费记录，而是创建新的退款交易。每个事务的借贷方向和金额必须满足账务平衡约束。

## 6. 审计与异步事件

### `audit_events`

```text
id
request_id
actor_type
actor_id
tenant_id
action
resource_type
resource_id
result                  success | denied | failed
before_json
after_json
ip_hash
user_agent_hash
reason
created_at
```

审计记录只允许追加，不提供普通业务删除接口。

### `outbox_events`

```text
id
event_type
aggregate_type
aggregate_id
payload_json
status                  pending | processing | published | failed
attempts
next_attempt_at
created_at
published_at
```

账务结算、审计写入和通知等异步动作通过 Outbox 触发，消费者必须支持幂等。

## 7. 数据库检查清单

- 所有租户表都有 `tenant_id` 和租户范围索引。
- 所有 Token、Session、账务流水和审计事件都有唯一性或幂等约束。
- 价格和账务记录保存版本或快照。
- 余额缓存可以重建，不能成为唯一事实来源。
- 数据库账号按服务拆分，Relay 不拥有用户和价格表写权限。
- 跨租户查询在测试中默认失败。
- 最新迁移版本为 `054_model_monitor_next_probe.sql`；应用启动使用事务和 PostgreSQL advisory lock 串行执行迁移。

### `email_templates` 与邮件功能设置

`platform_settings` 保存邮件总开关、模型状态开关、事件开关、SMTP 主机/端口/TLS、发件人信息、余额提醒阈值和充值地址；SMTP 密码只保存应用加密后的密文。`email_templates` 按 `event_code + language` 保存可启用的主题和 HTML 内容，035 迁移预置邮箱验证、密码重置、订阅、余额、限额、内容审计和运维等中英文模板，036 迁移初始化模型状态开关。

### `enterprise_verifications`

企业认证按租户保存企业名称、统一社会信用代码、营业执照元数据与密文、对公账户名称、开户行、银行账号密文、审核状态和审核人。服务端校验统一社会信用代码的允许字符、18 位长度和校验位；营业执照内容和银行账号不以明文返回用户端；用户端只返回银行账号脱敏值，管理员详情接口才返回完整银行账号。`046_enterprise_verification.sql` 创建该表和 `enterprise:read`、`enterprise:update` 权限。用户必须是当前租户的 active member；待审核申请不可重复提交，拒绝后可重新提交，已通过后不能再次提交。

### `payment_provider_configs` 与 `payment_orders`

`payment_provider_configs` 保存 `wechat`、`alipay`、`stripe`、`paypal` 四类支付方式的启用状态和加密配置 JSON。`recharge_rate` 是充值到账倍率，默认 1；`payment_method_types` 是 Stripe 可选的非敏感字段，只允许 `card`、`alipay`、`wechat_pay`，留空表示使用 Stripe Dashboard 动态支付方式；Apple Pay 和 Google Pay 不作为独立字段。Stripe 的 Publishable Key 可安全返回租户端，仅用于 Embedded Checkout 初始化；Secret Key、Webhook Secret、私钥、证书、API v3 Key 和 PayPal Client Secret 不会明文返回。Stripe 的 Webhook 路由是固定的 `/payments/webhooks/stripe`，成功/取消返回地址不落库，而是在创建 Checkout Session 时按充值页路由生成。`payment_orders` 保存租户、用户、支付方式、平台商户订单号、上游订单号、支付金额、到账额度、下单费率、Checkout 客户端密钥、订单状态、回调/支付页面信息和幂等键；官方回调或 PayPal Capture 验证支付金额后，按订单保存的到账额度调用账务服务入账，入账使用 `payment:credit:<order_id>` 幂等键。`047_payments.sql` 创建表、索引和 `payment:read`、`payment:update` 权限；`048_payment_idempotency_scope.sql` 将充值幂等键唯一性限定为 `(tenant_id, idempotency_key)`；`058_payment_recharge_rate.sql` 增加费率、到账额度和 Embedded Checkout 字段。

### `model_monitor_configs`

每个路由分组最多有一条模型监控配置。`selection_mode` 决定监控全部有效渠道模型映射，还是只监控 `model_monitor_config_models` 中显式选择的模型；`recent_request_limit` 仅允许 30、60、120。`primary_model_id` 由 `053_model_monitor_primary_model.sql` 增加，引用 `models.id` 并在模型删除时置空。`next_probe_at` 由 `054_model_monitor_next_probe.sql` 增加，用于记录下一次主动探测时间；主动监控启用或修改后立即执行，探测完成后按配置间隔重新安排，并加入最多约 10% 的随机抖动。服务端在新增或编辑时验证主模型属于当前监控范围；未选择主模型时自动选择范围内按名称排序的第一个模型。主模型字段只决定用户端分组健康卡默认展示的模型，不影响实际渠道调度、故障切换或计费。

### `api_endpoints`

`api_endpoints.base_url` 保存管理员配置的网关根地址，不保存 `/v1` 协议前缀。后台和公开设置接口根据该根地址派生 `openai_base_url`（根地址加 `/v1`）与 `anthropic_base_url`（根地址本身）。039 迁移创建终端表，040 迁移在无歧义时把历史 `/v1` 地址归一化为根地址；发生地址冲突时保留原记录，不自动删除数据。
