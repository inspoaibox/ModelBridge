# Linux Caddy + PM2 标准部署手册

本文是 AI Token Gateway 的标准 Linux 生产部署流程，使用 Caddy 自动 HTTPS 反向代理和 PM2 守护 Go 网关进程。适用于全新 Ubuntu 24.04 LTS 或 Debian 12 服务器。当前迁移目录最高版本为 `036_model_status_feature.sql`。

相关模板已随代码提供：

| 文件 | 部署位置 |
| --- | --- |
| deploy/caddy/Caddyfile | /etc/caddy/Caddyfile |
| deploy/pm2/start.sh | /opt/ai-token/current/deploy/pm2/start.sh |
| deploy/pm2/ecosystem.config.cjs | /opt/ai-token/current/deploy/pm2/ecosystem.config.cjs |
| deploy/pm2/ai-token-pm2.service | /etc/systemd/system/ai-token-pm2.service |

不要在 Shell 历史、Git、日志、工单、聊天、截图或备份文件中保存密码、Token、上游 Key、SMTP 密码、Pepper、MFA 密钥或管理员 TOTP Secret。

## 1. 目标架构

~~~text
Internet
  |
  v
Caddy :80/:443 (automatic TLS)
  |
  v
AI Token Gateway PM2 process :8080 (127.0.0.1 only)
  |                                   |
  v                                   v
PostgreSQL 16                     SMTP STARTTLS
~~~

建议起步：2 vCPU、4 GB RAM、40 GB SSD。视频、音频、上传、流式请求和大量使用记录需要按峰值并发与保留周期扩容。

发布前准备：

1. 域名，例如 gateway.example.com，A/AAAA 已指向服务器。
2. 对外仅开放 SSH、80 和 443。绝不开放 8080、5432。
3. 可用 SMTP STARTTLS 服务和测试邮箱。
4. 可信发布版本、SHA-256 或签名。
5. PostgreSQL 业务账号和备份空间。

## 2. 初始化服务器

使用具备 sudo 权限的账号：

~~~bash
sudo apt update
sudo apt -y full-upgrade
sudo apt install -y \
  ca-certificates curl gnupg git jq unzip \
  build-essential make gcc \
  postgresql postgresql-contrib \
  ufw fail2ban logrotate
sudo reboot
~~~

重连后设置防火墙和时钟：

~~~bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
sudo ufw status verbose
sudo timedatectl set-ntp true
timedatectl status
~~~

创建运行账号和目录：

~~~bash
sudo useradd --system --create-home --home-dir /var/lib/ai-token \
  --shell /usr/sbin/nologin ai-token
sudo install -d -o ai-token -g ai-token -m 0750 \
  /opt/ai-token/releases /var/lib/ai-token
sudo install -d -o root -g ai-token -m 0750 /etc/ai-token
sudo install -d -o caddy -g caddy -m 0750 /var/log/caddy
~~~

## 3. 安装 PostgreSQL

确认 PostgreSQL 至少为 16：

~~~bash
psql --version
sudo systemctl enable --now postgresql
sudo -u postgres psql
~~~

在 psql 中创建最小权限账号。使用 \\password 交互输入数据库密码：

~~~sql
CREATE ROLE ai_token LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
\\password ai_token
CREATE DATABASE ai_token OWNER ai_token ENCODING 'UTF8';
REVOKE ALL ON DATABASE ai_token FROM PUBLIC;
\\q
~~~

同机 PostgreSQL 必须只监听回环地址。postgresql.conf：

~~~conf
listen_addresses = '127.0.0.1,::1'
password_encryption = scram-sha-256
~~~

pg_hba.conf：

~~~conf
host    ai_token    ai_token    127.0.0.1/32    scram-sha-256
host    ai_token    ai_token    ::1/128         scram-sha-256
~~~

~~~bash
sudo systemctl reload postgresql
sudo -u postgres psql -d ai_token -c 'SELECT current_database(), current_user;'
~~~

远程数据库必须使用 TLS、IP 白名单和最小权限，不能暴露至公网。

## 4. 安装 Go、Node.js、PM2 和 Caddy

项目固定 Go 1.26.6。下载后验证官方校验和：

~~~bash
cd /tmp
curl -fLO https://go.dev/dl/go1.26.6.linux-amd64.tar.gz
curl -fLO https://go.dev/dl/go1.26.6.linux-amd64.tar.gz.sha256
sha256sum -c go1.26.6.linux-amd64.tar.gz.sha256
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.26.6.linux-amd64.tar.gz
echo 'export PATH=/usr/local/go/bin:$PATH' | sudo tee /etc/profile.d/go.sh >/dev/null
source /etc/profile.d/go.sh
go version
~~~

安装 Node.js 22 LTS：

~~~bash
curl -fsSL https://deb.nodesource.com/setup_22.x -o /tmp/nodesource-22.sh
sudo bash /tmp/nodesource-22.sh
sudo apt install -y nodejs
sudo npm install --global pm2
node --version
npm --version
command -v pm2
~~~

安装 Caddy 官方软件源：

~~~bash
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/gpg.key \
  | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt \
  | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update
sudo apt install -y caddy
caddy version
~~~

如果服务器已运行 Nginx 或 Apache，占用 80/443 的服务必须先停用：

~~~bash
sudo systemctl disable --now nginx 2>/dev/null || true
sudo systemctl disable --now apache2 2>/dev/null || true
~~~

## 5. 获取和构建发布版本

以下使用版本 YYYY-MM-DD.N。替换为真实来源和版本：

~~~bash
export RELEASE=YYYY-MM-DD.N
sudo -u ai-token -H mkdir -p /opt/ai-token/releases/$RELEASE
sudo -u ai-token -H git clone --depth 1 --branch "$RELEASE" \
  https://YOUR_GIT_HOST/YOUR_ORG/ai-token.git \
  /opt/ai-token/releases/$RELEASE
~~~

若使用压缩包，先核验 SHA-256 或签名。发布包不得包含 .env、node_modules、dist、本地缓存、日志、数据库转储或密钥。

~~~bash
sudo -u ai-token -H bash
export PATH=/usr/local/go/bin:$PATH
cd /opt/ai-token/releases/$RELEASE

go mod download
go test ./...
CGO_ENABLED=1 go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...

cd web
npm ci
npm run lint
npm run typecheck
npm test
npm audit --omit=dev --registry=https://registry.npmjs.org
cd ..

mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/ai-token ./cmd/server
chmod 0755 deploy/pm2/start.sh
test -f web/dist/index.html
exit
~~~

## 6. 完整生产环境文件

创建 /etc/ai-token/ai-token.env：

~~~bash
sudoedit /etc/ai-token/ai-token.env
sudo chown root:ai-token /etc/ai-token/ai-token.env
sudo chmod 0640 /etc/ai-token/ai-token.env
~~~

填写以下模板。所有 REPLACE 值必须替换，且不得复用开发环境密钥：

~~~dotenv
APP_ENV=production
HTTP_ADDR=127.0.0.1:8080
HTTP_READ_TIMEOUT=60s
HTTP_WRITE_TIMEOUT=15m
HTTP_IDLE_TIMEOUT=2m
WEB_DIR=/opt/ai-token/current/web
MIGRATIONS_DIR=/opt/ai-token/current/migrations

DATABASE_URL=postgres://ai_token:REPLACE_DATABASE_PASSWORD@127.0.0.1:5432/ai_token?sslmode=disable

TOKEN_PEPPER=REPLACE_RANDOM_48_BYTE_HEX
SESSION_PEPPER=REPLACE_DIFFERENT_RANDOM_48_BYTE_HEX
MFA_ENCRYPTION_KEY=REPLACE_RANDOM_32_BYTE_HEX
COOKIE_SECURE=true
SESSION_TTL=12h
LOGIN_MAX_FAILURES=5
LOGIN_FAILURE_WINDOW=15m
LOGIN_LOCK_DURATION=15m

CORS_ALLOWED_ORIGINS=https://gateway.example.com
TRUSTED_PROXY_CIDRS=127.0.0.1/32,::1/128

REGISTRATION_ENABLED=false
# Must be true when REGISTRATION_ENABLED=true in production.
REGISTRATION_EMAIL_VERIFICATION_REQUIRED=true
PUBLIC_BASE_URL=https://gateway.example.com
SMTP_ADDR=smtp.example.com:587
SMTP_FROM=no-reply@example.com
SMTP_USERNAME=REPLACE_SMTP_USERNAME
SMTP_PASSWORD=REPLACE_SMTP_PASSWORD
~~~

生成密钥：

~~~bash
openssl rand -hex 48
openssl rand -hex 32
~~~

规则：

| 变量 | 生产规则 |
| --- | --- |
| TOKEN_PEPPER、SESSION_PEPPER | 至少 32 字符，且必须不同。轮换会失效现有 Token 或 Session。 |
| MFA_ENCRYPTION_KEY | 32 字节 hex 或 base64。丢失后现有 TOTP 密钥无法解密。 |
| CORS_ALLOWED_ORIGINS | 精确 HTTPS 来源，不使用通配符。 |
| TRUSTED_PROXY_CIDRS | 同机 Caddy 使用 127.0.0.1/32,::1/128；绝不填写 0.0.0.0/0。 |
| REGISTRATION_ENABLED | 必须显式设置，建议 false。开启公开注册前部署邮箱验证、Captcha 和 WAF。 |
| REGISTRATION_EMAIL_VERIFICATION_REQUIRED | 生产公开注册必须为 `true`；否则应用拒绝启动。开发环境可按本机测试需要关闭。 |
| PUBLIC_BASE_URL | HTTPS 站点地址，用于密码重置和邮箱验证链接；必须与实际访问域名一致。 |
| SMTP | 必须同时配置地址、发件人并支持 STARTTLS；应用拒绝明文 SMTP。公开注册开启时必须可投递测试邮件。 |
| HTTP_WRITE_TIMEOUT | 默认 15 分钟，覆盖长响应和视频创建；不得设置为无限时长。 |

生产模式缺少上述关键变量会拒绝启动。

SMTP 也可以在管理员登录后通过“系统设置”写入数据库。环境变量适合作为首次启动兜底，但公开注册上线前仍必须在真实域名下完成一次注册、验证链接、重发邮件和登录验收。系统设置只返回 SMTP 密码是否已配置，不返回密码明文。

## 7. Caddy 配置

复制模板并替换域名与邮箱：

~~~bash
sudo install -m 0644 /opt/ai-token/releases/$RELEASE/deploy/caddy/Caddyfile \
  /etc/caddy/Caddyfile
sudoedit /etc/caddy/Caddyfile
sudo caddy fmt --overwrite /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
sudo systemctl enable --now caddy
sudo systemctl status caddy --no-pager
~~~

Caddy 自动申请和续期证书。模板将客户端伪造的 X-Forwarded-For 替换为 Caddy 的直连来源地址；应用仅信任同机 Caddy 的回环 CIDR。

如果 Caddy 前还有 CDN 或负载均衡器，必须先在 Caddy 层正确恢复真实客户端 IP，再将实际 Caddy 地址或网段填写至 TRUSTED_PROXY_CIDRS。不要将公网网段加入该变量。

## 8. PM2 进程守护

切换 current 链接并安装 PM2 模板：

~~~bash
sudo ln -sfn /opt/ai-token/releases/$RELEASE /opt/ai-token/current
sudo chown -h ai-token:ai-token /opt/ai-token/current
sudo chmod 0755 /opt/ai-token/current/deploy/pm2/start.sh
sudo install -m 0644 \
  /opt/ai-token/current/deploy/pm2/ai-token-pm2.service \
  /etc/systemd/system/ai-token-pm2.service
sudo systemctl daemon-reload
~~~

以 ai-token 账号启动 PM2。PM2 配置不会保存任何生产密钥，启动脚本只读取权限为 root:ai-token 0640 的环境文件：

~~~bash
sudo -u ai-token -H env PM2_HOME=/var/lib/ai-token/.pm2 \
  pm2 startOrReload /opt/ai-token/current/deploy/pm2/ecosystem.config.cjs \
  --update-env
sudo -u ai-token -H env PM2_HOME=/var/lib/ai-token/.pm2 pm2 save
sudo -u ai-token -H env PM2_HOME=/var/lib/ai-token/.pm2 pm2 status
sudo -u ai-token -H env PM2_HOME=/var/lib/ai-token/.pm2 pm2 logs ai-token --lines 100
~~~

让 systemd 在重启后恢复 PM2：

~~~bash
sudo -u ai-token -H env PM2_HOME=/var/lib/ai-token/.pm2 pm2 kill
sudo systemctl enable --now ai-token-pm2
sudo systemctl status ai-token-pm2 --no-pager
sudo -u ai-token -H env PM2_HOME=/var/lib/ai-token/.pm2 pm2 status
~~~

检查：

~~~bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS https://gateway.example.com/healthz
sudo journalctl -u ai-token-pm2 -n 100 --no-pager
sudo -u ai-token -H env PM2_HOME=/var/lib/ai-token/.pm2 pm2 monit
~~~

## 9. 创建首个管理员

管理员不通过公开注册创建：

~~~bash
sudo -u ai-token -H bash
set -a
source /etc/ai-token/ai-token.env
set +a
read -rp 'Administrator email: ' ADMIN_EMAIL
read -rsp 'Administrator password: ' ADMIN_PASSWORD
echo
export ADMIN_EMAIL ADMIN_PASSWORD
go run /opt/ai-token/current/cmd/bootstrap-admin
unset ADMIN_PASSWORD
exit
~~~

若工具输出 TOTP Secret，只能在安全终端中导入认证器并保存恢复方案。

## 10. 管理员角色与公开注册

首个管理员由 `bootstrap-admin` 创建为受保护的 `platform_owner`。客户用户不能由管理员后台创建，统一从前端公开注册；公开注册只建立租户所有者、默认项目和预付账户，不会自动授予平台管理员角色。

登录后进入“用户与组织管理”可以查看用户及组织 ID；进入“平台角色与权限”可以：

1. 由 `platform_owner` 创建、编辑和停用自定义平台角色，并从系统已登记权限中选择权限。
2. 将已经注册、状态为正常的用户绑定为一个或多个平台角色。
3. 通过 TOTP Step-up 确认角色写入、角色停用和管理员绑定。

`platform_owner` 定义不可编辑或停用，最后一个有效平台管理员不能被移除。自定义角色的权限或状态改变后，受影响管理员的旧后台 Session 会立即失效，必须重新登录。

当需要开放客户注册时，先在“系统设置”填写 `PUBLIC_BASE_URL`、SMTP 地址、发件人、SMTP 用户名和密码并保存，然后在环境文件中设置：

~~~dotenv
REGISTRATION_ENABLED=true
REGISTRATION_EMAIL_VERIFICATION_REQUIRED=true
~~~

重载应用后执行以下验收：

1. 使用未注册邮箱注册，确认返回待验证提示，不能直接登录。
2. 使用真实邮件中的一次性链接验证，确认账号变为可登录状态。
3. 使用“验证邮箱”页面输入邮箱请求重发，确认旧链接失效、新链接可用且接口不泄露邮箱是否存在。
4. 为测试账号绑定和解除自定义平台角色，确认旧管理员 Session 被拒绝，重新登录后权限与角色一致。
5. 关闭公开注册时确认注册接口不可用；不要把关闭注册误认为 SMTP 故障。

## 11. 上线验收

~~~bash
curl -fsS https://gateway.example.com/healthz
curl -I https://gateway.example.com/
curl -i https://gateway.example.com/src/App.tsx
curl -i -X OPTIONS https://gateway.example.com/v1/models \
  -H 'Origin: https://attacker.example' \
  -H 'Access-Control-Request-Method: GET'
sudo -u postgres psql -d ai_token -c \
  'SELECT max(version), count(*) FROM schema_migrations;'
~~~

数据库验收结果应包含最新迁移 `036_model_status_feature.sql`；如果版本低于 036，不得开启公开注册、模型状态或邮件功能。

验收至少包含：

1. 首页、模型广场、登录、密码重置、404 和移动端。
2. 管理员的 TOTP、渠道、模型、分组、Token、用户、价格、财务、使用记录和审计。
3. 租户的资料、Token 创建/撤销、分组模型可见性、IP 白名单和账单。
4. 最低权限 Token 的文本、Embedding、图片、音频、视频接口。
5. SMTP 测试邮件、一次性重置链接、旧 Session 失效和新密码登录。
6. 备份和恢复演练。

对于使用记录中状态为 `settlement_pending` 的请求，必须核对上游账单或 Usage 后，通过 `POST /admin/v1/usage/{requestID}/settle` 补录真实计量；不可直接按空用量提交。

## 12. 日常操作

查看状态：

~~~bash
sudo systemctl status caddy ai-token-pm2 --no-pager
sudo -u ai-token -H env PM2_HOME=/var/lib/ai-token/.pm2 pm2 status
sudo -u ai-token -H env PM2_HOME=/var/lib/ai-token/.pm2 pm2 logs ai-token --lines 200
sudo journalctl -u caddy -u ai-token-pm2 -n 200 --no-pager
~~~

每日备份：

~~~bash
sudo install -d -o postgres -g postgres -m 0700 /var/backups/ai-token
sudo -u postgres pg_dump -Fc -d ai_token \
  -f /var/backups/ai-token/ai-token-$(date +%F).dump
~~~

恢复演练：

~~~bash
sudo -u postgres createdb ai_token_restore
sudo -u postgres pg_restore -d ai_token_restore \
  /var/backups/ai-token/ai-token-YYYY-MM-DD.dump
sudo -u postgres psql -d ai_token_restore -c 'SELECT count(*) FROM schema_migrations;'
sudo -u postgres dropdb ai_token_restore
~~~

## 13. 更新和回滚

更新：

~~~bash
export RELEASE=YYYY-MM-DD.N
sudo -u postgres pg_dump -Fc -d ai_token \
  -f /var/backups/ai-token/pre-$RELEASE.dump

# 下载到 /opt/ai-token/releases/$RELEASE，并执行第 5 节全部测试和构建
sudo ln -sfn /opt/ai-token/releases/$RELEASE /opt/ai-token/current
sudo chmod 0755 /opt/ai-token/current/deploy/pm2/start.sh
sudo -u ai-token -H env PM2_HOME=/var/lib/ai-token/.pm2 \
  pm2 startOrReload /opt/ai-token/current/deploy/pm2/ecosystem.config.cjs \
  --update-env
sudo -u ai-token -H env PM2_HOME=/var/lib/ai-token/.pm2 pm2 save
curl -fsS https://gateway.example.com/healthz
~~~

仅当数据库迁移向后兼容时才可仅回滚应用：

~~~bash
sudo ln -sfn /opt/ai-token/releases/PREVIOUS_RELEASE /opt/ai-token/current
sudo -u ai-token -H env PM2_HOME=/var/lib/ai-token/.pm2 \
  pm2 startOrReload /opt/ai-token/current/deploy/pm2/ecosystem.config.cjs \
  --update-env
curl -fsS https://gateway.example.com/healthz
~~~

若迁移不兼容，停止 PM2，恢复已验证的数据库备份，切回旧版本，再完成验收。不要手工删除 schema_migrations。

## 14. 常见故障

| 症状 | 处理 |
| --- | --- |
| Caddy 无法申请证书 | 检查 DNS、80/443、防火墙、Caddy 日志和域名是否被其他服务占用。 |
| PM2 反复重启 | 查看 pm2 logs 和环境文件权限；确认 current/bin/ai-token 存在。 |
| Token IP 白名单失败 | 检查 TRUSTED_PROXY_CIDRS、Caddy header_up 和应用是否只监听 127.0.0.1。 |
| 密码重置 503 | 检查 SMTP STARTTLS、PUBLIC_BASE_URL、SMTP_FROM 和环境变量。 |
| 注册不可用 | 检查 REGISTRATION_ENABLED、REGISTRATION_EMAIL_VERIFICATION_REQUIRED、数据库迁移版本和系统设置；生产关闭公开注册是预期行为。 |
| 注册后无法登录 | 检查账号是否仍为 `pending`，重新发送验证邮件并检查 SMTP 投递、PUBLIC_BASE_URL 和链接有效期。 |
| 页面 502 | 检查 pm2 status、127.0.0.1:8080 healthz、Caddy 日志。 |
| 迁移失败 | 停止 PM2、查看 PostgreSQL 日志、从备份恢复，不要修改迁移记录。 |

每月升级系统并复验：

~~~bash
sudo apt update && sudo apt -y full-upgrade
go version
node --version
caddy version
sudo caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
sudo -u postgres psql -d ai_token -c 'VACUUM (ANALYZE);'
~~~
