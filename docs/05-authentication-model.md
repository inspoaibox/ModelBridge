# 认证模型

## 1. 三类身份

平台区分三类身份，不能混用：

```text
Browser Session       管理端和租户控制台
Downstream API Token  下游调用模型 API
Service Identity      内部服务之间调用
```

每次认证都应产生明确的 `audience`：

```text
admin
console
relay
```

`billing` 和 `audit` 是权限资源，不是当前浏览器可登录的独立 audience。

一个身份只能在允许的 audience 中使用。

## 2. 管理端和用户端登录

推荐使用服务端保存的 opaque Session：

```text
浏览器 -> 登录接口
       -> 服务端验证凭据和 MFA
       -> 创建随机 Session
       -> 数据库只保存 Session 摘要
       -> 浏览器保存 HttpOnly Cookie
```

Cookie 示例属性：

```text
Secure
HttpOnly
SameSite=Lax 或 Strict
Path=/
```

管理端和用户端使用不同 Cookie 名称，例如：

```text
admin_session
console_session
```

建议策略：

- 登录成功后轮换 Session，防止 Session fixation。
- 修改密码、MFA、角色或安全设置后撤销旧 Session。
- 管理端使用较短的空闲超时和绝对超时。
- 支持查看和撤销当前账号的其他会话。
- 登录失败、密码重置和 MFA 验证都要限速。
- 生产公开注册要求邮箱验证；新账号在验证前为 `pending`，不能建立控制台 Session。
- 统一返回登录失败信息，避免暴露账号是否存在。

## 3. 管理员 MFA

TOTP 由“系统设置 → 功能开关”控制，默认关闭。主开关开启后，租户用户和管理员可各自在个人资料中选择绑定或解除绑定；个人绑定仅保护该账号登录，不会强制其他账号。

管理员全局 MFA 由管理员中心中的独立策略控制，默认关闭；开启前所有有效管理员必须已完成个人绑定。优先支持：

```text
WebAuthn / Passkey
TOTP
一次性恢复码
```

管理员全局 MFA 开启后，高风险操作使用实时 TOTP step-up。每次敏感写请求携带 `X-MFA-Code`，验证码不持久化；失败达到阈值后短时锁定。不能由前端传入一个布尔值表示“已验证”。

## 4. 下游 API Token

Token 只用于 `api.example.com` 的模型调用，不用于网页控制台登录。

建议格式：

```text
YOUR_GATEWAY_TOKEN
```

创建时：

1. 使用密码学安全随机数生成完整 Token。
2. 保存 `token_prefix`。
3. 使用服务端 pepper 计算摘要并保存 `token_hash`。
4. 完整 Token 只返回一次。
5. 记录创建者、租户、项目、权限范围和过期时间。

调用时：

```text
Authorization: Bearer YOUR_GATEWAY_TOKEN
```

服务端只根据 Token 摘要查找记录，然后检查：

```text
status
expires_at
tenant_id
project_id
creator account and membership status
creator project role
scopes
allowed_models
allowed_ips / allowed_domains
rate_limit
```

不接受 Query 参数中的 Token，也不将 Token 写入 URL。

## 5. 公开注册与邮箱验证

`REGISTRATION_ENABLED` 仅在首次启动时写入“开放用户注册”的初始状态；之后由管理员“系统设置 → 功能开关”实时控制，无需重启。邮件总开关和“邮箱验证码”开关同时开启时，注册流程为：

```text
注册 -> 创建 pending 用户和租户资源 -> SMTP 发送一次性链接
     -> POST /console/v1/auth/email/verify -> 用户变为 active -> 允许登录
```

验证令牌只保存带 pepper 的摘要，30 分钟过期且只能使用一次。SMTP 必须使用 STARTTLS；Captcha、Bot 防护和 WAF 需要在应用前的边缘层部署。

## 5. Service Identity

内部服务不共享管理员账号、数据库超级账号或上游 API Key。

每个服务使用独立身份和 audience：

```text
relay -> usage.write
billing-worker -> ledger.write
audit-writer -> audit.append
admin-api -> tenant.manage
```

内部 Token 建议短时有效，并支持密钥轮换。高敏感部署可以进一步使用 mTLS 或工作负载身份。

## 6. 认证上下文

认证中间件输出统一上下文：

```text
AuthContext {
  PrincipalID
  PrincipalType
  Audience
  TenantID
  ProjectIDs
  Roles
  Scopes
  AuthStrength
  SessionID
  TokenID
  RequestID
}
```

业务代码不得自行解析 Cookie、Authorization Header 或 JWT Claims。所有身份信息通过统一的 `AuthContext` 获取。

## 7. 错误与响应

对外只暴露稳定错误码：

```text
AUTH_REQUIRED
AUTH_INVALID
AUTH_EXPIRED
AUTH_REVOKED
PERMISSION_DENIED
RESOURCE_NOT_FOUND
RATE_LIMITED
```

对于不属于当前租户的资源，通常返回 `RESOURCE_NOT_FOUND`，避免通过 `403` 暴露资源存在性。内部日志保留真实拒绝原因。

## 8. 安全事件

以下事件进入安全事件流和审计日志：

```text
login_succeeded
login_failed
mfa_added
mfa_removed
session_revoked
role_changed
api_token_created
api_token_revoked
api_token_expired
step_up_succeeded
step_up_failed
cross_scope_access_denied
```
