# Linux 生产部署与运维手册

本文用于在全新 Linux 服务器部署 AI Token Gateway。基线为 Ubuntu 24.04 LTS、PostgreSQL 16、Nginx、systemd。生产由 Nginx 终止 TLS，应用仅监听本机回环地址。

不要在 Shell 历史、Git、工单、聊天、截图或日志中记录数据库密码、Token、上游 API Key、SMTP 密码、Pepper、MFA 密钥、管理员密码或 TOTP Secret。

## 1. 架构和发布前准备

~~~text
Internet -> Nginx :443 -> AI Token Gateway :8080 (127.0.0.1) -> PostgreSQL
                                                  |
                                                  -> SMTP STARTTLS
~~~

建议起步配置为 2 vCPU、4 GB 内存、40 GB SSD。音视频、上传和高并发流式业务需要按实际容量独立评估。

准备：

1. 域名，例如 gateway.example.com，A/AAAA 记录已指向服务器。
2. 仅开放 TCP 80、443 和 SSH 管理端口；不开放 8080 或 PostgreSQL 5432。
3. 可用 SMTP STARTTLS 服务和测试邮箱。
4. 受信任的发布源、版本号、校验和或签名。
5. 一个非超级用户 PostgreSQL 业务账号。

## 2. 初始化 Ubuntu 或 Debian

以 sudo 账号执行：

~~~bash
sudo apt update
sudo apt -y full-upgrade
sudo apt install -y \
  ca-certificates curl gnupg git jq unzip \
  build-essential make gcc \
  nginx certbot python3-certbot-nginx \
  postgresql postgresql-contrib \
  ufw fail2ban logrotate
sudo reboot
~~~

重新连接后启用防火墙和时间同步：

~~~bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow OpenSSH
sudo ufw allow 'Nginx Full'
sudo ufw enable
sudo ufw status verbose
sudo timedatectl set-ntp true
timedatectl status
~~~

创建专用运行账号和目录：

~~~bash
sudo useradd --system --create-home --home-dir /var/lib/ai-token \
  --shell /usr/sbin/nologin ai-token
sudo install -d -o ai-token -g ai-token -m 0750 \
  /opt/ai-token/releases /opt/ai-token/bin /var/lib/ai-token
sudo install -d -o root -g ai-token -m 0750 /etc/ai-token
~~~

## 3. PostgreSQL

确认 PostgreSQL 至少为 16：

~~~bash
psql --version
sudo systemctl enable --now postgresql
sudo systemctl status postgresql --no-pager
sudo -u postgres psql
~~~

在交互式 psql 中创建最小权限账号。使用 \\password 录入密码，避免密码进入历史记录：

~~~sql
CREATE ROLE ai_token LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
\\password ai_token
CREATE DATABASE ai_token OWNER ai_token ENCODING 'UTF8';
REVOKE ALL ON DATABASE ai_token FROM PUBLIC;
\\q
~~~

同机数据库保持本地监听。确认 postgresql.conf：

~~~conf
listen_addresses = '127.0.0.1,::1'
password_encryption = scram-sha-256
~~~

在 pg_hba.conf 中限制为本机 SCRAM：

~~~conf
host    ai_token    ai_token    127.0.0.1/32    scram-sha-256
host    ai_token    ai_token    ::1/128         scram-sha-256
~~~

~~~bash
sudo systemctl reload postgresql
sudo -u postgres psql -d ai_token -c 'SELECT current_database(), current_user;'
~~~

远程 PostgreSQL 必须启用 TLS、来源 IP 白名单和最小权限；不要将 5432 暴露到公网。

## 4. 安装 Go 和 Node.js

项目固定 Go 工具链为 1.26.6。下载后必须核验官方 SHA-256：

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

安装 Node.js 22 LTS。应使用组织批准的软件源；下例使用 NodeSource：

~~~bash
curl -fsSL https://deb.nodesource.com/setup_22.x -o /tmp/nodesource-22.sh
sudo bash /tmp/nodesource-22.sh
sudo apt install -y nodejs
node --version
npm --version
~~~

build-essential 和 gcc 是 go test -race 的必需依赖，不应省略。

## 5. 获取、构建和校验发布版本

以下使用版本 YYYY-MM-DD.N。替换发布源和版本号：

~~~bash
export RELEASE=YYYY-MM-DD.N
sudo -u ai-token -H mkdir -p /opt/ai-token/releases/$RELEASE
sudo -u ai-token -H git clone --depth 1 --branch "$RELEASE" \
  https://YOUR_GIT_HOST/YOUR_ORG/ai-token.git \
  /opt/ai-token/releases/$RELEASE
~~~

若使用压缩包，先校验 SHA-256 或签名，再解压到发布目录。发布包不得包含 .env、node_modules、dist、本地缓存、日志、数据库转储或管理员凭据。

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
npm audit --omit=dev
cd ..

mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/ai-token ./cmd/server
test -f web/dist/index.html
exit
~~~

任一步失败都不得继续发布。

## 6. 生产环境变量

创建环境文件：

~~~bash
sudoedit /etc/ai-token/ai-token.env
sudo chown root:ai-token /etc/ai-token/ai-token.env
sudo chmod 0640 /etc/ai-token/ai-token.env
~~~

模板：

~~~dotenv
APP_ENV=production
HTTP_ADDR=127.0.0.1:8080
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
PUBLIC_BASE_URL=https://gateway.example.com
SMTP_ADDR=smtp.example.com:587
SMTP_FROM=no-reply@example.com
SMTP_USERNAME=REPLACE_SMTP_USERNAME
SMTP_PASSWORD=REPLACE_SMTP_PASSWORD
~~~

生成随机值时，立即存入安全密码管理器：

~~~bash
openssl rand -hex 48
openssl rand -hex 32
~~~

关键要求：

| 配置 | 要求 |
| --- | --- |
| TOKEN_PEPPER、SESSION_PEPPER | 至少 32 字符且必须不同；轮换会影响现有 Token 或 Session。 |
| MFA_ENCRYPTION_KEY | 32 字节 hex 或 base64；丢失后无法解密现有 TOTP 密钥。 |
| CORS_ALLOWED_ORIGINS | 精确 HTTPS 来源，逗号分隔，不使用通配符。 |
| TRUSTED_PROXY_CIDRS | 只填写真实反向代理地址或网段，绝不能填写 0.0.0.0/0。 |
| REGISTRATION_ENABLED | 生产必须显式设置；建议关闭，启用前部署邮箱验证、Captcha 和 WAF 限流。 |
| PUBLIC_BASE_URL | HTTPS 站点地址，用于生成密码重置链接。 |
| SMTP | 必须支持 STARTTLS，先用测试邮箱验证投递。 |

生产模式缺少数据库、密钥、CORS、注册开关、SMTP、公共地址或受信任代理配置时，应用将拒绝启动。

## 7. systemd

使用 current 符号链接支持原子升级：

~~~bash
sudo ln -sfn /opt/ai-token/releases/$RELEASE /opt/ai-token/current
sudo chown -h ai-token:ai-token /opt/ai-token/current
~~~

创建 /etc/systemd/system/ai-token.service：

~~~ini
[Unit]
Description=AI Token Gateway
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
Type=simple
User=ai-token
Group=ai-token
WorkingDirectory=/opt/ai-token/current
EnvironmentFile=/etc/ai-token/ai-token.env
ExecStartPre=/usr/bin/test -f /opt/ai-token/current/web/dist/index.html
ExecStart=/opt/ai-token/current/bin/ai-token
Restart=on-failure
RestartSec=5
TimeoutStartSec=60
TimeoutStopSec=20
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/var/lib/ai-token
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
CapabilityBoundingSet=
AmbientCapabilities=
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
~~~

~~~bash
sudo systemctl daemon-reload
sudo systemctl enable --now ai-token
sudo systemctl status ai-token --no-pager
sudo journalctl -u ai-token -n 100 --no-pager
curl -fsS http://127.0.0.1:8080/healthz
~~~

应用会在首次启动时在单个数据库事务中执行未应用迁移。启动前必须备份数据库。

## 8. Nginx 和 TLS

创建 /etc/nginx/sites-available/ai-token：

~~~nginx
server {
    listen 80;
    listen [::]:80;
    server_name gateway.example.com;
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://$host$request_uri; }
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name gateway.example.com;

    ssl_certificate /etc/letsencrypt/live/gateway.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/gateway.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_timeout 1d;
    ssl_session_cache shared:TLS:10m;

    client_max_body_size 50m;
    proxy_connect_timeout 10s;
    proxy_send_timeout 120s;
    proxy_read_timeout 300s;

    location / {
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:8080;
    }

    location /v1/ {
        proxy_http_version 1.1;
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:8080;
    }
}
~~~

启用并申请证书：

~~~bash
sudo ln -sfn /etc/nginx/sites-available/ai-token /etc/nginx/sites-enabled/ai-token
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t
sudo systemctl reload nginx
sudo certbot --nginx -d gateway.example.com
sudo systemctl enable --now certbot.timer
sudo certbot renew --dry-run
~~~

若 Nginx 位于外部负载均衡器之后，只将实际负载均衡器 CIDR 加入 TRUSTED_PROXY_CIDRS；应用只在直连代理可信时解析 X-Forwarded-For。

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

若工具显示 TOTP Secret，只能在安全终端中导入认证器并保存恢复方案。不要复制到聊天、工单或日志。

## 10. 上线验收

~~~bash
curl -fsS https://gateway.example.com/healthz
curl -I https://gateway.example.com/
curl -i https://gateway.example.com/admin/v1/not-found
curl -i -X OPTIONS https://gateway.example.com/v1/models \
  -H 'Origin: https://attacker.example' \
  -H 'Access-Control-Request-Method: GET'
sudo -u postgres psql -d ai_token -c \
  'SELECT max(version), count(*) FROM schema_migrations;'
~~~

必须完成：

1. 验证首页、模型广场、登录、注册关闭状态、密码重置和 404。
2. 用真实管理员验证 TOTP、改密、渠道、模型、分组、Token、用户、价格、财务、用量和审计。
3. 用真实租户验证个人资料、Token 创建/撤销、分组模型可见性、IP 白名单和账单。
4. 以最低权限 Token 验证模型、文本、Embedding、图片、音频和视频接口。
5. 验证 CORS 拒绝非白名单来源、源码路径返回 404、安全响应头存在。
6. 用测试邮箱验证 SMTP、一次性重置链接、过期、旧 Session 失效和新密码登录。

## 11. 备份、监控和恢复

每日备份，且定期做恢复演练：

~~~bash
sudo install -d -o postgres -g postgres -m 0700 /var/backups/ai-token
sudo -u postgres pg_dump -Fc -d ai_token \
  -f /var/backups/ai-token/ai-token-$(date +%F).dump

sudo -u postgres createdb ai_token_restore
sudo -u postgres pg_restore -d ai_token_restore \
  /var/backups/ai-token/ai-token-YYYY-MM-DD.dump
sudo -u postgres psql -d ai_token_restore -c 'SELECT count(*) FROM schema_migrations;'
sudo -u postgres dropdb ai_token_restore
~~~

监控 systemd、healthz、Nginx 5xx、证书到期、PostgreSQL、备份、渠道失败率、熔断、余额不足、媒体任务积压、认证限流和 SMTP 失败。日志不得含完整 Token、密码、上游 Key 或重置链接。

## 12. 升级

所有升级先在预发布环境验证：

~~~bash
export RELEASE=YYYY-MM-DD.N
sudo -u postgres pg_dump -Fc -d ai_token \
  -f /var/backups/ai-token/pre-$RELEASE.dump

sudo -u ai-token -H mkdir -p /opt/ai-token/releases/$RELEASE
# 下载并校验新版本，然后执行第 5 节的完整构建检查

sudo ln -sfn /opt/ai-token/releases/$RELEASE /opt/ai-token/current
sudo systemctl restart ai-token
sudo systemctl status ai-token --no-pager
curl -fsS https://gateway.example.com/healthz
~~~

数据库迁移在启动时事务化执行。升级前必须阅读新迁移，确认锁表、数据量和回滚影响。

## 13. 回滚

仅当新迁移向后兼容时，才能仅回滚应用：

~~~bash
sudo ln -sfn /opt/ai-token/releases/PREVIOUS_RELEASE /opt/ai-token/current
sudo systemctl restart ai-token
curl -fsS https://gateway.example.com/healthz
~~~

迁移不向后兼容时，停止应用、使用已验证备份恢复数据库、切回旧发布目录，再执行完整验收。恢复前保留故障数据库副本。

## 14. 故障排查和例行维护

| 症状 | 检查顺序 |
| --- | --- |
| 服务无法启动 | journalctl、生产环境变量、web/dist、数据库连接和文件权限。 |
| 页面空白或 404 | WEB_DIR、npm build、Nginx proxy_pass 和 systemd current 链接。 |
| IP 白名单全部拒绝 | TRUSTED_PROXY_CIDRS、X-Forwarded-For、Nginx 是否同机监听。 |
| 密码重置 503 | SMTP、PUBLIC_BASE_URL、环境文件权限和测试邮箱。 |
| 注册不可用 | REGISTRATION_ENABLED；生产关闭公开注册是预期行为。 |
| 迁移失败 | 停止服务、查看 PostgreSQL 日志、从备份恢复；不要手工删 schema_migrations。 |

每月执行：

~~~bash
sudo apt update && sudo apt -y full-upgrade
go version
node --version
sudo certbot renew --dry-run
sudo -u postgres psql -d ai_token -c 'VACUUM (ANALYZE);'
~~~

升级操作系统、Go、Node、PostgreSQL 或依赖后，必须在预发布环境重跑质量检查、浏览器验收、数据库恢复演练和真实上游调用验证。
