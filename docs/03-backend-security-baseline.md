# 后台安全基线

## 1. 安全边界

推荐的最小部署边界：

```text
Internet
  -> WAF / Load Balancer
      -> Admin API      仅管理端入口
      -> User API       用户控制台入口
      -> Relay API      下游模型调用入口

内部网络
  -> PostgreSQL
  -> Redis
  -> Queue
  -> KMS / Secret Manager
```

数据库、Redis、消息队列和密钥服务不直接暴露公网。Relay Service 不应具备管理用户、修改价格和退款权限。

## 2. 管理员账户

- 管理员 MFA 由后台安全设置控制，默认关闭；开启后必须使用 MFA。
- 高风险操作进行 step-up authentication。
- 支持管理员会话主动失效和全局踢出。
- Session 设置 `Secure`、`HttpOnly` 和合适的 `SameSite`。
- 管理端 Cookie 与用户端 Cookie 使用不同名称和作用域。
- 登录、找回密码、MFA 验证和角色变更全部限速并写审计。
- 禁止共享管理员账号。
- 超级管理员账号只用于紧急处置，日常使用低权限角色。

## 3. Web 和 API 防护

- 全站 HTTPS，生产环境禁止明文回退。
- Cookie 认证接口启用 CSRF 防护。
- CORS 使用明确的来源白名单，不使用通配符。
- 所有输入执行 schema 校验、长度限制和类型校验。
- 限制请求体大小、最大输入 Token、最大并发和 Streaming 空闲时长。
- 对错误响应做统一处理，不返回 SQL、上游凭据或内部堆栈。
- 管理接口、健康检查、指标接口和调试接口分开暴露。
- 管理接口增加来源限制或 VPN / Zero Trust 接入层。

## 4. 上游渠道和密钥

- 上游 API Key 使用 KMS、Vault 或等效密钥管理系统加密保存。
- 数据库、审计日志、错误日志和监控标签均不得出现完整密钥。
- 只有 Relay Service 在请求执行的最短时间内读取解密值。
- 渠道密钥支持轮换、禁用和版本追踪。
- 自定义上游地址必须执行 SSRF 防护：限制协议、域名和端口，阻止私网地址、回环地址、云元数据地址和 DNS 重绑定。
- 每个渠道配置独立连接超时、响应超时、重试次数和熔断策略。

## 5. 计费与防滥用

- 请求开始时创建幂等的额度预占记录。
- 请求结束时以标准化 Usage 事件结算。
- Streaming 中断也必须进入结算流程。
- 价格未配置时拒绝计费请求，不得静默按零价格放行。
- 重试、超时、回调和消息重复投递不能重复扣费。
- 余额、充值、消费和退款使用不可变账务流水。
- 对用户、Token、租户、IP、模型分别配置 RPM、TPM、并发和日限额。
- 对异常高频、模型滥用、批量创建 Token 和批量导出进行告警或冻结。

## 6. 敏感数据和日志

默认不保存完整 Prompt/Response。必须保存时：

- 明确租户级开关和保留期限。
- 对内容、Header、Token、Cookie 和 Authorization 做脱敏。
- 敏感日志独立权限访问。
- 导出文件短期有效并可撤销。
- 生产日志不允许记录原始上游响应中的密钥或个人信息。

基础请求日志建议包含：

```text
request_id
tenant_id
project_id
token_id_prefix
model
channel_id
status_code
input_tokens
output_tokens
reserved_amount
settled_amount
latency_ms
created_at
```

## 7. 审计与告警

审计事件至少分为：

```text
auth
permission
tenant
token
channel
price
billing
data_export
security_policy
```

每条事件记录操作者、身份类型、目标资源、动作、结果、请求 ID、IP、设备、时间和必要的变更摘要。历史审计事件只允许追加，不允许普通管理员修改或删除。

至少配置以下告警：

- 管理员异地或异常设备登录
- 连续登录失败或 MFA 失败
- 管理员角色授予
- 价格、渠道密钥和风控策略变更
- 大额退款或人工余额调整
- 单 Token 突发高并发
- 跨租户访问被拒绝次数异常
- 大量 Prompt/Response 导出

## 8. 发布前安全门禁

每次发布至少执行：

1. 角色矩阵自动化测试。
2. 水平越权和垂直越权测试。
3. Token 撤销、过期和轮换测试。
4. 账务幂等、流式中断和重试结算测试。
5. SSRF、CSRF、CORS、输入校验测试。
6. 密钥扫描和依赖漏洞扫描。
7. 生产配置检查，确认调试接口、默认账号和宽泛 CORS 已关闭。
8. 审计事件完整性检查。

安全验收可以参考 OWASP API Security Top 10 和 OWASP ASVS，重点覆盖对象级授权、功能级授权、资源消耗、SSRF、安全配置错误和敏感业务流程滥用。
