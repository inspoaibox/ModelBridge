-- Email is opt-in. SMTP credentials remain encrypted by the application;
-- this migration only creates non-secret configuration and template storage.
INSERT INTO platform_settings (key, value)
VALUES
    ('email_enabled', 'false'),
    ('email_verification_enabled', 'true'),
    ('email_password_reset_enabled', 'true'),
    ('email_subscription_enabled', 'true'),
    ('email_low_balance_alert_enabled', 'false'),
    ('email_recharge_success_enabled', 'false'),
    ('email_usage_limit_alert_enabled', 'false'),
    ('email_content_audit_enabled', 'false'),
    ('email_account_disabled_enabled', 'false'),
    ('email_cyber_policy_enabled', 'false'),
    ('email_operations_enabled', 'false'),
    ('email_balance_threshold', '0'),
    ('email_recharge_url', ''),
    ('smtp_host', ''),
    ('smtp_port', '587'),
    ('smtp_from_email', ''),
    ('smtp_from_name', ''),
    ('smtp_tls', 'true')
ON CONFLICT (key) DO NOTHING;

CREATE TABLE IF NOT EXISTS email_templates (
    id uuid PRIMARY KEY,
    event_code text NOT NULL,
    language text NOT NULL CHECK (language IN ('zh', 'en')),
    subject text NOT NULL,
    html_body text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_by uuid REFERENCES users(id),
    updated_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (event_code, language)
);

CREATE INDEX IF NOT EXISTS email_templates_event_idx
    ON email_templates (event_code, language, enabled);

INSERT INTO email_templates (id, event_code, language, subject, html_body)
VALUES
    ('11111111-1111-4111-8111-111111112001', 'email_verification', 'zh', '{{site_name}} 邮箱验证码', '<p>您好，</p><p>请点击下面的按钮验证邮箱：</p><p><a href="{{verification_url}}">验证邮箱</a></p><p>链接有效期为 30 分钟。</p>'),
    ('11111111-1111-4111-8111-111111112002', 'email_verification', 'en', '{{site_name}} email verification', '<p>Hello,</p><p>Click the link below to verify your email address:</p><p><a href="{{verification_url}}">Verify email</a></p><p>This link expires in 30 minutes.</p>'),
    ('11111111-1111-4111-8111-111111112003', 'password_reset', 'zh', '{{site_name}} 密码重置', '<p>您好，</p><p>请点击下面的按钮重置密码：</p><p><a href="{{reset_url}}">重置密码</a></p><p>链接有效期为 30 分钟。</p>'),
    ('11111111-1111-4111-8111-111111112004', 'password_reset', 'en', '{{site_name}} password reset', '<p>Hello,</p><p>Click the link below to reset your password:</p><p><a href="{{reset_url}}">Reset password</a></p><p>This link expires in 30 minutes.</p>'),
    ('11111111-1111-4111-8111-111111112005', 'notification_email_verification', 'zh', '{{site_name}} 邮箱验证通知', '<p>您的邮箱验证已完成。</p>'),
    ('11111111-1111-4111-8111-111111112006', 'notification_email_verification', 'en', '{{site_name}} email verification notice', '<p>Your email address has been verified.</p>'),
    ('11111111-1111-4111-8111-111111112007', 'subscription_started', 'zh', '{{site_name}} 订阅开通成功', '<p>您的订阅已开通成功。</p><p>套餐：{{plan_name}}</p>'),
    ('11111111-1111-4111-8111-111111112008', 'subscription_started', 'en', '{{site_name}} subscription activated', '<p>Your subscription is now active.</p><p>Plan: {{plan_name}}</p>'),
    ('11111111-1111-4111-8111-111111112009', 'subscription_expiring', 'zh', '{{site_name}} 订阅到期提醒', '<p>您的订阅即将到期，请及时续费。</p><p>到期时间：{{expires_at}}</p>'),
    ('11111111-1111-4111-8111-111111112010', 'subscription_expiring', 'en', '{{site_name}} subscription expiring', '<p>Your subscription will expire soon.</p><p>Expires at: {{expires_at}}</p>'),
    ('11111111-1111-4111-8111-111111112011', 'low_balance', 'zh', '{{site_name}} 余额不足提醒', '<p>您的账户余额已低于提醒阈值。</p><p>当前余额：{{balance}}</p><p><a href="{{recharge_url}}">前往充值</a></p>'),
    ('11111111-1111-4111-8111-111111112012', 'low_balance', 'en', '{{site_name}} low balance alert', '<p>Your account balance is below the alert threshold.</p><p>Current balance: {{balance}}</p><p><a href="{{recharge_url}}">Recharge now</a></p>'),
    ('11111111-1111-4111-8111-111111112013', 'recharge_success', 'zh', '{{site_name}} 余额充值成功', '<p>您的余额充值已成功到账。</p><p>充值金额：{{amount}}</p><p>当前余额：{{balance}}</p>'),
    ('11111111-1111-4111-8111-111111112014', 'recharge_success', 'en', '{{site_name}} recharge successful', '<p>Your recharge was completed successfully.</p><p>Amount: {{amount}}</p><p>Current balance: {{balance}}</p>'),
    ('11111111-1111-4111-8111-111111112015', 'usage_limit_alert', 'zh', '{{site_name}} 账号限额告警', '<p>您的账号已接近或达到使用限额。</p><p>使用量：{{usage}}</p>'),
    ('11111111-1111-4111-8111-111111112016', 'usage_limit_alert', 'en', '{{site_name}} usage limit alert', '<p>Your account is near or has reached its usage limit.</p><p>Usage: {{usage}}</p>'),
    ('11111111-1111-4111-8111-111111112017', 'content_audit_violation', 'zh', '{{site_name}} 内容审计违规提醒', '<p>您的请求触发了内容审计规则，请检查调用内容。</p><p>时间：{{event_time}}</p>'),
    ('11111111-1111-4111-8111-111111112018', 'content_audit_violation', 'en', '{{site_name}} content policy notice', '<p>Your request triggered a content policy rule.</p><p>Time: {{event_time}}</p>'),
    ('11111111-1111-4111-8111-111111112019', 'account_disabled', 'zh', '{{site_name}} 账号已禁用', '<p>您的账号已被暂时禁用。如有疑问，请联系平台管理员。</p>'),
    ('11111111-1111-4111-8111-111111112020', 'account_disabled', 'en', '{{site_name}} account disabled', '<p>Your account has been disabled. Contact the platform administrator if you need help.</p>'),
    ('11111111-1111-4111-8111-111111112021', 'cyber_policy_notice', 'zh', '{{site_name}} Cyber policy notice', '<p>平台安全策略发生变化，请登录控制台查看详情。</p>'),
    ('11111111-1111-4111-8111-111111112022', 'cyber_policy_notice', 'en', '{{site_name}} Cyber policy notice', '<p>The platform security policy has changed. Sign in to the console for details.</p>'),
    ('11111111-1111-4111-8111-111111112023', 'ops_alert', 'zh', '{{site_name}} 运维告警', '<p>平台运维告警：{{message}}</p>'),
    ('11111111-1111-4111-8111-111111112024', 'ops_alert', 'en', '{{site_name}} operations alert', '<p>Platform operations alert: {{message}}</p>'),
    ('11111111-1111-4111-8111-111111112025', 'ops_daily_report', 'zh', '{{site_name}} 运维定时报表', '<p>运维定时报表已生成。</p><p>报告时间：{{event_time}}</p>'),
    ('11111111-1111-4111-8111-111111112026', 'ops_daily_report', 'en', '{{site_name}} operations daily report', '<p>The operations daily report is ready.</p><p>Report time: {{event_time}}</p>')
ON CONFLICT (event_code, language) DO NOTHING;
