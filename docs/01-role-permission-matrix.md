# 角色与权限矩阵

## 1. 设计目标

本平台同时服务平台运营人员和下游租户。权限系统必须满足：

- 管理端和用户端身份体系分离。
- 平台权限与租户权限分离。
- 权限同时限制“能做什么”和“能操作哪些资源”。
- 所有高风险操作可审计、可追溯，必要时需要二次确认或双人审批。
- 后端每个接口都独立鉴权，前端隐藏菜单不属于安全控制。

## 2. 身份边界

建议使用独立的站点和 API 路由：

```text
admin.example.com    平台管理端
console.example.com  租户控制台
api.example.com      下游模型 API
```

管理端和用户端使用不同的：

- 登录入口和 Session Cookie 名称
- Session 存储命名空间
- Token audience
- CORS allowlist
- API 中间件

管理端 Session 不能调用用户端接口，用户端 Session 不能调用管理端接口。服务端仍然必须对每个请求重新检查权限，不能以域名作为唯一安全边界。

## 3. 平台角色

当前版本提供平台角色管理 UI/API。平台角色只能由具备 `platform_owner` 的管理员创建、编辑和停用，也只能由平台所有者把已注册用户绑定为平台管理员；公开注册永远只创建租户用户，不会创建平台角色或管理员。首个 `platform_owner` 由 `bootstrap-admin` 创建。

| 角色 | 典型职责 | 默认不可执行 |
|---|---|---|
| `platform_owner` | 平台全局配置、角色授权、紧急处置 | 角色定义本身不可编辑；应作为极少数 break-glass 账号 |
| 自定义平台角色 | 按权限集合分配渠道、账务、审计或用户管理职责 | 未勾选的权限；高风险写操作仍需 Step-up |

原则：

- 一个员工可以拥有多个低风险角色，但高风险权限应单独授予。
- 超级管理员不作为日常运营角色使用。
- 高风险操作应支持双人审批或至少二次认证。
- 角色变更本身必须产生审计记录。
- 停用角色不能导致平台没有任何有效管理员；最后一个管理员或最后一个角色身份会被保护。
- 具备普通 `user:update` 权限的管理员不能修改 `platform_owner` 账号；只有另一名有效平台所有者可以执行该操作。
- `pending` 用户必须通过邮箱验证变为 `active`；后台状态修改不能绕过邮箱验证。

## 4. 租户角色

| 角色 | 资源范围 | 典型权限 |
|---|---|---|
| `tenant_owner` | 当前租户 | 成员、项目、Token、套餐、账单、租户设置 |
| `tenant_admin` | 当前租户 | 成员、项目、Token、模型权限、限流 |
| `developer` | 被授权项目 | 创建、编辑、暂停、启用、终止和删除自己的 Token，查看项目用量 |
| `viewer` | 被授权项目 | 只读查看模型、用量和账单 |

租户角色永远不能访问：

- 其他租户的用户、项目、Token、日志和账单
- 平台渠道的上游密钥
- 平台全局价格表的编辑权限
- 平台其他租户的运营数据

## 5. 权限命名

使用 `resource:action` 命名，资源范围另行判断：

```text
tenant:read
tenant:update
member:invite
member:remove
token:create
token:pause
token:revoke
token:read_secret
project:update
model:use
usage:read
model:status:read
billing:read
billing:refund
channel:read
channel:update
channel:read_secret
price:publish
enterprise:read
enterprise:update
payment:read
payment:update
security:read
security:update
role:read
role:update
user:freeze
audit:read
audit:export
```

平台管理员的 Token 权限仅用于查看和暂停（`token:read`、`token:pause`）；`token:update` 与 `token:revoke` 只用于租户用户管理自己创建的令牌。

默认拒绝。未配置的权限不能因为前端存在按钮、接口参数或数据库默认值而自动放行。

## 6. API 鉴权流程

每个请求按以下顺序处理：

```text
解析身份
  -> 校验 Session 或 API Token
  -> 检查身份状态和过期时间
  -> 解析租户、项目和资源范围
  -> 检查 action 权限
  -> 检查资源归属和策略条件
  -> 执行业务操作
  -> 写入审计结果
```

请求中的 `tenant_id`、`owner_id` 只能作为筛选条件，不能作为权限来源。当前租户应从已验证的身份上下文中取得。

## 7. 下游 API Token

下游 Token 与网页 Session 完全分开。每个 Token 至少包含：

```text
id
tenant_id
project_id
created_by
scopes
allowed_models
allowed_ips / allowed_domains
creator account and project membership status
rate_limit
expires_at
last_used_at
status
```

建议：

- Token 使用高熵随机值生成。
- 数据库只保存不可逆摘要或带服务端 pepper 的摘要。
- 完整 Token 只在创建成功时显示一次。
- 支持编辑、暂停、启用、终止、软删除、过期、轮换和批量冻结。
- 日志只记录 Token 前缀和内部 ID。
- `token:read_secret` 不授予任何普通后台角色。
- 创建者账号停用、锁定、租户成员暂停/移除、项目降为只读或移除项目授权后，相关 Token 不能继续解析；项目权限变更会撤销项目内由该用户创建的 Token。

## 8. 高风险操作

当前实现对管理端高风险写操作要求实时 TOTP Step-up。请求在已认证的管理员 Session 上增加：

```http
X-MFA-Code: 123456
```

缺少验证码返回 `403 STEP_UP_REQUIRED`，验证码错误返回 `401 MFA_CODE_INVALID`；同一管理员的敏感操作 MFA 失败达到阈值后会短时返回 `429 MFA_STEP_UP_THROTTLED`。验证码只在当前请求中校验，不写入数据库或审计日志。生产服务必须为每个执行这些操作的管理员绑定个人 TOTP。

以下操作至少需要重新输入 MFA 或一次性确认码：

- 修改模型价格或费率版本
- 充值、赠送额度、人工调整余额
- 修改渠道上游地址或凭据
- 修改用户状态、撤销/调整 Token 和系统设置
- 创建、编辑、停用平台角色，绑定或解绑平台管理员
- 批量删除、批量冻结
- 导出 Prompt、Response 或用户数据（当前导出功能暂不开放）
- 修改全局限流、风控和审计策略

账务操作不允许直接修改余额字段。充值、消费、退款和人工调整都必须写入不可变流水。
