# 授权中间件与接口保护

本文是授权设计与当前实现说明。代码已经实现 audience、Session/Token 状态、权限、租户/项目范围、网络白名单、平台角色管理和审计基础；前端隐藏菜单不属于安全控制，后端会重复校验所有资源边界。

## 1. 路由分区

```text
/admin/*    平台管理接口
/console/*  租户控制台接口
/v1/*       下游模型兼容接口
/internal/* 服务间接口
```

路由分区只是第一层保护。每个 Handler 仍需要执行认证、权限、资源范围和状态校验。

## 2. 中间件顺序

推荐顺序：

```text
Request ID
  -> TLS / Origin 检查
  -> 请求体和 Header 限制
  -> 认证
  -> 账号和 Session 状态检查
  -> 速率与并发限制
  -> 权限检查
  -> 租户与资源范围检查
  -> Step-up 检查
  -> Handler
  -> 审计和指标
```

模型转发接口的顺序略有不同：

```text
Request ID
  -> API Token 认证
  -> Token 状态和权限
  -> 租户余额预占
  -> 模型和渠道路由
  -> 上游请求
  -> Usage 标准化
  -> 结算
  -> Usage 和审计事件
```

## 3. 授权接口

实现一个统一的授权入口：

```text
Authorize(ctx, action, resource)
```

其中：

```text
action   = "channel:update"
resource = Channel{id, tenant_id, status}
```

授权判断至少包含：

```text
身份是否存在
audience 是否匹配
身份是否处于 active 状态
角色或 scope 是否包含 action
资源是否属于当前 tenant/project
资源状态是否允许当前操作
是否需要 step-up
```

默认拒绝，只有明确配置的权限才允许。

## 4. 典型接口规则

| 接口 | 身份 | 权限 | 额外检查 |
|---|---|---|---|
| `GET /admin/v1/channels` | 管理 Session | `channel:read` | 只返回脱敏密钥预览 |
| `POST /admin/v1/channels/{channelID}/sync-account` | 管理 Session | `channel:read` | 手动查询余额/倍率；失败只更新旁路状态，不需要 `channel:update` 或 Step-up |
| `POST /admin/v1/channels/discover-models` | 管理 Session | `channel:update` | 临时使用提交的 Key 探测模型，不写入数据库 |
| `POST /admin/v1/channels` | 管理 Session | `channel:update` | 不能提交明文密钥到日志 |
| `PUT /admin/v1/channels/{channelID}` | 管理 Session | `channel:update` | 密钥轮换也必须脱敏记录 |
| `POST /admin/v1/channels/{channelID}/pause` | 管理 Session | `channel:update` | 仅变更状态 |
| `POST /admin/v1/channels/{channelID}/enable` | 管理 Session | `channel:update` | 仅变更状态 |
| `DELETE /admin/v1/channels/{channelID}` | 管理 Session | `channel:update` | 软删除并吊销活跃密钥 |
| `POST /admin/prices/publish` | 管理 Session | `price:publish` | MFA、价格版本、审计 |
| `POST /admin/v1/roles` | `platform_owner` 管理 Session | `role:update` | TOTP Step-up、已登记权限、审计 |
| `PUT /admin/v1/users/{userID}/roles` | `platform_owner` 管理 Session | `role:update` | TOTP Step-up、最后管理员保护 |
| `POST /admin/billing/refunds` | 管理 Session | `billing:refund` | Step-up、幂等键 |
| `GET /console/usage` | 用户 Session | `usage:read` | 当前租户和项目范围 |
| `GET /console/v1/tenants/{tenantID}/model-status` | 用户 Session | `model:status:read` | 当前租户路径；只返回分组、模型和脱敏健康摘要 |
| `POST /console/v1/tenants/{tenantID}/tokens` | 用户 Session | `token:create` | 只能创建当前用户在授权项目中的 Token |
| `PUT /console/v1/tenants/{tenantID}/tokens/{tokenID}` | 用户 Session | `token:update` | 只能编辑本人创建的 Token |
| `POST /console/v1/tenants/{tenantID}/tokens/{tokenID}/pause` | 用户 Session | `token:update` | 暂停本人创建的 active Token |
| `POST /console/v1/tenants/{tenantID}/tokens/{tokenID}/resume` | 用户 Session | `token:update` | 启用本人创建的 disabled Token |
| `POST /console/v1/tenants/{tenantID}/tokens/{tokenID}/terminate` | 用户 Session | `token:revoke` | 永久终止本人创建的 Token |
| `DELETE /console/v1/tenants/{tenantID}/tokens/{tokenID}` | 用户 Session | `token:revoke` | 软删除并终止本人创建的 Token |
| `POST /v1/chat/completions` | API Token | `model:use` | 模型白名单、余额、限流 |
| `POST /internal/usage-events` | Service Identity | `usage.write` | 只接受可信服务调用 |

## 5. 资源查询规范

### 列表接口

所有列表查询必须由服务端注入范围条件：

```text
tenant_id = AuthContext.TenantID
project_id IN AuthContext.ProjectIDs
```

客户端传入的 `tenant_id` 只能用于一致性校验，不能扩大范围。

### 单资源接口

禁止先按 ID 查询，再在后续代码中补权限判断。应该在查询时绑定范围：

```sql
SELECT *
FROM api_tokens
WHERE id = :token_id
  AND tenant_id = :current_tenant_id
  AND project_id = ANY(:allowed_project_ids)
  AND deleted_at IS NULL;
```

### 更新接口

使用字段 allowlist，禁止直接将请求 JSON 映射到数据库模型。用户不能通过额外字段修改：

```text
tenant_id
created_by
owner_id
status
balance
role_code
price_version_id
```

这些字段必须由专用业务流程更新。

## 6. 高风险接口

高风险接口统一使用：

```text
RequirePermission(action)
RequireStepUp()
AuditMutation()
```

当管理员全局 MFA 策略开启时，Step-up 采用每次敏感请求的 `X-MFA-Code`，而不是可长期复用的前端布尔值；验证码不会持久化，失败达到阈值后按管理员身份短时锁定。

高风险操作建议包含：

- 幂等键，防止客户端重试重复执行。
- 变更前后摘要。
- 操作原因。
- 审批人或审批单号。
- 事务内的业务变化和审计 Outbox 事件。

## 7. 拒绝策略

```text
未登录                  -> 401 AUTH_REQUIRED
Session/Token 无效      -> 401 AUTH_INVALID
身份已过期              -> 401 AUTH_EXPIRED
权限不足                -> 403 PERMISSION_DENIED
资源不在当前范围        -> 404 RESOURCE_NOT_FOUND
超过速率或并发          -> 429 RATE_LIMITED
缺少高风险确认          -> 403 STEP_UP_REQUIRED
```

内部日志必须记录拒绝原因、主体、资源范围和 Request ID，但对外错误信息保持稳定，避免泄露资源信息。

## 8. 自动化测试要求

每个受保护接口至少覆盖：

- 未登录访问。
- 普通用户访问管理接口。
- 租户 A 访问租户 B 的资源。
- 同租户无项目权限的用户访问项目资源。
- 低权限角色调用高权限动作。
- Token 已撤销、已过期和模型不在白名单。
- 缺少 MFA 或 Step-up 的高风险操作。
- 重放相同幂等键。
- 修改请求中的 `tenant_id`、`owner_id`、`role_code` 和 `balance`。

权限测试应使用“允许矩阵”和“拒绝矩阵”双向生成，避免只测试成功路径。
