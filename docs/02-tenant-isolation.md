# 租户隔离与数据访问规则

## 1. 核心原则

租户隔离是后端数据访问规则，不是前端页面规则。

所有业务资源都应归属于明确的租户，必要时再归属于项目：

```text
tenant
  -> member
  -> project
       -> api_token
       -> usage_event
       -> request_log
       -> billing_entry
```

平台级资源，例如渠道、上游凭据、全局价格模板，归属于平台，不应直接暴露给租户。

## 2. 身份上下文

请求通过认证后，服务端生成不可由客户端修改的上下文：

```text
principal_id
principal_type       platform_user | tenant_user | api_token | service
tenant_id
project_ids
roles
scopes
auth_strength
request_id
```

业务层只能使用这个上下文进行授权。不要从请求体、查询参数或隐藏表单字段中直接读取当前租户。

## 3. 数据库规则

每张租户业务表都必须有 `tenant_id`，并建立组合索引：

```text
(tenant_id, id)
(tenant_id, created_at)
(tenant_id, status)
```

推荐采用三层防护：

1. Repository 层所有查询强制接收 `TenantScope`。
2. Service 层执行资源归属和角色校验。
3. PostgreSQL 使用 Row-Level Security 作为额外隔离层。

禁止以下模式：

```go
db.First(&item, request.ID)
```

应当显式绑定当前租户：

```go
db.Where("tenant_id = ? AND id = ?", scope.TenantID, request.ID).First(&item)
```

## 4. 缓存、队列和对象存储

租户 ID 必须进入所有跨请求存储的 key：

```text
tenant:{tenant_id}:project:{project_id}:usage:{date}
tenant:{tenant_id}:token:{token_id}
```

队列消息必须携带 `tenant_id`、`project_id` 和 `request_id`。消费者不能仅凭对象 ID 读取或更新资源。

对象存储中的日志、导出文件和账单文件应使用租户隔离的前缀，并通过短期签名地址访问。签名地址不得永久有效。

## 5. 日志访问

按默认最小暴露原则：

- 租户用户只能看自己租户、自己项目允许范围内的用量和账单。
- 普通用户默认只能看聚合用量，不直接查看完整 Prompt/Response。
- 管理员查看敏感请求内容时必须具备单独权限，并记录访问原因。
- 审计日志不能被业务管理员删除或覆盖。
- 日志导出必须再次鉴权，并设置过期时间。

## 6. 服务间权限

建议给服务分配独立身份：

| 服务 | 可读 | 可写 | 明确禁止 |
|---|---|---|---|
| Admin API | 平台配置、用户和账务 | 管理业务数据、审计事件 | 直接执行上游请求 |
| User API | 当前租户资源 | 当前租户 Token 和项目设置 | 访问平台密钥和其他租户 |
| Relay Service | 渠道运行配置、加密凭据 | Usage 事件、请求状态 | 修改价格、用户角色、退款 |
| Billing Worker | 价格版本、Usage 事件 | 账务流水和结算结果 | 修改渠道密钥和用户权限 |
| Audit Writer | 必要的操作者上下文 | 追加审计事件 | 修改或删除历史审计记录 |

服务使用独立数据库账号，不能共享一个拥有全部表写权限的账号。

## 7. 必须覆盖的隔离测试

- 修改 URL、Query、JSON 中的 ID，不能读取其他租户资源。
- 租户管理员不能调用平台管理员接口。
- 被移除的成员立即失去项目和 Token 访问权。
- 撤销 Token 后，已有缓存不能继续调用。
- 缓存、队列重试和导出文件不会串租户。
- 聚合统计不会因为分组缺失而泄露单个租户数据。
- 账单、退款和用量查询不能跨租户。
- 管理员查看敏感日志必须留下完整审计事件。

