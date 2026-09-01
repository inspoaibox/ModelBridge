# Linux Caddy + PM2 标准部署手册

本文是 AI Token Gateway 的标准 Linux 生产部署流程，使用 Caddy 自动 HTTPS 反向代理和 PM2 守护 Go 网关进程。适用于全新 Ubuntu 24.04 LTS 或 Debian 12 服务器。当前迁移目录最高版本为 `036_model_status_feature.sql`。

> **给第一次部署的用户**
>
> 推荐只阅读并执行本文，不要同时执行 `docs/09-linux-deployment.md`。
> 本文按从零开始的顺序编排，请按章节一步一步执行，不要把整篇文档一次性粘贴到终端。
>
> 代码块右上角的类型决定执行方式：
>
> | 类型 | 用途 | 是否粘贴到 `root@服务器:~#` 后执行 |
> | --- | --- | --- |
> | `bash` | Linux 终端命令 | 是，按行执行 |
> | `sql` | PostgreSQL 的 `postgres=#` 提示符内的 SQL | 否，先执行进入 psql 的命令 |
> | `conf` | PostgreSQL 配置文件内容 | 否，写入指定配置文件 |
> | `dotenv` | AI Token 环境文件内容 | 否，写入 `/etc/ai-token/ai-token.env` |
> | `caddyfile` | Caddy 配置文件内容 | 否，写入 `/etc/caddy/Caddyfile` |
>
> 看到 `REPLACE_...`、`gateway.example.com` 或 `YOUR_...` 时，必须替换成自己的值。
> 文档中的 `$RELEASE` 是当前发布版本变量，不是要原样输入的文字。

### 终端身份和编辑器

本文的普通命令可以在 root 终端执行，也可以在具有 sudo 权限的账号下执行。
命令中的 `sudo -u ai-token`、`sudo -u postgres` 表示临时使用对应系统账号执行，不需要先切换账号。

遇到 `sudoedit` 或 `nano` 时，打开的是文件编辑器，不是在执行命令：

1. 把需要写入文件的内容粘贴到编辑器中。
2. nano 保存：按 `Ctrl+O`，回车确认文件名。
3. nano 退出：按 `Ctrl+X`。
4. 保存后再执行文档中的下一条 `bash` 命令。

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
~~~

## 3. 安装 PostgreSQL

确认 PostgreSQL 至少为 16：

~~~bash
psql --version
sudo systemctl enable --now postgresql
~~~

### 配置 PostgreSQL 监听地址

下面两段内容是 PostgreSQL 配置文件内容，不是 Linux 命令。
如果把它们直接粘贴到 `root@服务器:~#` 后面，就会出现
`-bash: listen_addresses: command not found`。

先查询实际配置文件路径：

~~~bash
sudo -u postgres psql -tAc "SHOW config_file"
sudo -u postgres psql -tAc "SHOW hba_file"
~~~

第一条命令通常输出 `postgresql.conf` 的路径，第二条通常输出
`pg_hba.conf` 的路径。使用输出的真实路径打开文件，例如：

~~~bash
sudo nano /etc/postgresql/16/main/postgresql.conf
~~~

在 `postgresql.conf` 中找到同名配置项。如果原来有被 `#` 注释的配置，
删除开头的 `#` 并修改；如果没有，就添加下面两行。每个配置项只保留一份有效行：

~~~conf
listen_addresses = '127.0.0.1,::1'
password_encryption = scram-sha-256
~~~

按 `Ctrl+O` 保存，回车确认，再按 `Ctrl+X` 退出。

打开上面第二条命令输出的 `pg_hba.conf` 路径，例如：

~~~bash
sudo nano /etc/postgresql/16/main/pg_hba.conf
~~~

在文件末尾添加下面两行：

~~~conf
host    ai_token    ai_token    127.0.0.1/32    scram-sha-256
host    ai_token    ai_token    ::1/128         scram-sha-256
~~~

保存并退出 nano 后，回到 Linux 终端执行。这里必须使用 `restart`，
因为 `listen_addresses` 不是仅 reload 就能生效的配置：

~~~bash
sudo systemctl restart postgresql
~~~

### 创建应用数据库和账号

先在 Linux 终端执行：

~~~bash
sudo -u postgres psql
~~~

看到 `postgres=#` 后，再执行下面的 SQL。不要把 SQL 直接粘贴到
`root@服务器:~#` 后面：

~~~sql
CREATE ROLE ai_token LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
\password ai_token
CREATE DATABASE ai_token OWNER ai_token ENCODING 'UTF8';
REVOKE ALL ON DATABASE ai_token FROM PUBLIC;
\q
~~~

其中 `\password ai_token` 会单独询问数据库密码。输入密码时终端不会显示字符，
输入完成后按回车即可。执行 `\q` 后会回到 Linux 终端。

最后验证应用账号可以连接：

~~~bash
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
~~~

如果服务器已运行 Nginx 或 Apache，占用 80/443 的服务，必须先停用。
这一段应在安装 Caddy 前执行：

~~~bash
sudo systemctl disable --now nginx 2>/dev/null || true
sudo systemctl disable --now apache2 2>/dev/null || true
~~~

现在安装 Caddy。此时系统会创建 `caddy` 运行账号，再创建访问日志目录：

~~~bash
sudo apt install -y caddy
sudo install -d -o caddy -g caddy -m 0750 /var/log/caddy
caddy version
~~~

## 5. 获取和构建发布版本

这一节的目标只有两件事：

1. 从 GitHub 下载项目代码到服务器。
2. 把前端和后端编译成可运行程序。

下面所有代码块都是 `bash` 命令，可以复制到 Linux 终端执行。

### 第 1 步：下载项目代码

当前项目 Git 仓库地址为：

```text
https://github.com/inspoaibox/ModelBridge.git
```

`RELEASE` 只是服务器上本次发布目录的名字，可以使用日期加序号。
例如今天第一次部署可使用 `2026-09-01-01`。它不是 Git 标签，也不需要在
GitHub 上预先创建。

当前仓库没有发布标签，因此首次部署使用 `main` 分支。下面整段命令可直接复制
到 Linux 终端：

~~~bash
export RELEASE=2026-09-01-01
export GIT_BRANCH=main
export REPO_URL=https://github.com/inspoaibox/ModelBridge.git

sudo -u ai-token -H mkdir -p /opt/ai-token/releases/$RELEASE
sudo -u ai-token -H git clone --depth 1 --branch "$GIT_BRANCH" \
  "$REPO_URL" \
  /opt/ai-token/releases/$RELEASE

sudo -u ai-token -H git -C /opt/ai-token/releases/$RELEASE rev-parse HEAD
~~~

最后一条会输出一长串提交编号。把它记录到部署记录中，用于将来确认当前服务器
运行的是哪一版代码。

如果你以后创建了正式 Git 标签，例如 `v1.0.0`，只需要把
`export GIT_BRANCH=main` 改成 `export GIT_BRANCH=v1.0.0` 即可。
不要复制文档中的 `YOUR_...`、`YYYY-MM-DD.N` 之类占位符。

### 第 2 步：执行检查并编译

下面整段命令会切换到 `ai-token` 运行账号，下载依赖、检查代码、编译前端和后端。
看到提示符变成类似 `ai-token@服务器:~$` 后属于正常情况；最后的 `exit`
会自动返回 root 终端。

~~~bash
sudo -u ai-token -H env RELEASE="$RELEASE" bash
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

上面命令的作用：

| 命令 | 在做什么 |
| --- | --- |
| `go mod download` | 下载 Go 后端需要的依赖。 |
| `go test ./...` | 运行后端自动测试。 |
| `go test -race ./...` | 检查常见并发问题，耗时会比普通测试长。 |
| `go vet ./...` | 检查明显的 Go 代码错误。 |
| `govulncheck` | 检查 Go 依赖中的已知安全漏洞。 |
| `npm ci` | 根据锁定版本下载前端依赖。 |
| `npm run lint`、`npm run typecheck`、`npm test` | 检查并构建 React 前端；`npm test` 会生成 `web/dist`。 |
| `npm audit` | 检查前端生产依赖漏洞。 |
| `go build ... -o bin/ai-token` | 生成实际运行的后端程序 `/opt/ai-token/releases/本次版本/bin/ai-token`。 |
| `chmod 0755 .../start.sh` | 让 PM2 启动脚本可以执行。 |

任何一条命令出现错误时，先停下来处理错误，不要跳过后面的检查继续上线。
压缩包部署时，也必须先把压缩包解压到
`/opt/ai-token/releases/$RELEASE`，再从本节“第 2 步”开始执行。

## 6. 完整生产环境文件

创建 `/etc/ai-token/ai-token.env`。先在 Linux 终端执行：

~~~bash
sudo nano /etc/ai-token/ai-token.env
~~~

进入 nano 后，把文件内容替换为下面的 `dotenv` 模板。
`dotenv` 不是 Bash 命令。所有 `REPLACE_...`、域名和邮箱都必须替换：

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

在 nano 中按 `Ctrl+O` 保存，回车确认文件名，再按 `Ctrl+X` 退出。
回到 Linux 终端后，设置文件所有者和权限：

~~~bash
sudo chown root:ai-token /etc/ai-token/ai-token.env
sudo chmod 0640 /etc/ai-token/ai-token.env
~~~

生成随机密钥。前三条命令分别输出一个随机值，请依次填入环境文件对应的
`TOKEN_PEPPER`、`SESSION_PEPPER` 和 `MFA_ENCRYPTION_KEY`。第三条命令
生成的 64 个十六进制字符用于 `MFA_ENCRYPTION_KEY`：

~~~bash
openssl rand -hex 48
openssl rand -hex 48
openssl rand -hex 32
~~~

为了避免数据库密码中的 `@`、`#`、`?` 等特殊字符破坏 `DATABASE_URL`，
建议数据库密码也使用十六进制随机值，例如执行：

~~~bash
openssl rand -hex 24
~~~

将这个值用于 PostgreSQL 的 `\password ai_token`，并填入
`DATABASE_URL` 中的 `REPLACE_DATABASE_PASSWORD`。所有密钥和密码都应
立即保存到安全的密码管理器，不要提交到 Git。

如果前面保存的只是模板，此时必须再次打开环境文件，把上述四个随机值、
真实数据库密码、域名、邮箱和 SMTP 配置填入对应位置：

~~~bash
sudo nano /etc/ai-token/ai-token.env
~~~

保存按 `Ctrl+O`，回车确认；退出按 `Ctrl+X`。退出后再次确认文件权限：

~~~bash
sudo chown root:ai-token /etc/ai-token/ai-token.env
sudo chmod 0640 /etc/ai-token/ai-token.env
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

复制模板并编辑域名与邮箱。先在 Linux 终端执行：

~~~bash
sudo install -m 0644 /opt/ai-token/releases/$RELEASE/deploy/caddy/Caddyfile \
  /etc/caddy/Caddyfile
sudo nano /etc/caddy/Caddyfile
~~~

进入 nano 后只修改以下内容：

1. 将 `email ops@example.com` 改成接收证书通知的真实邮箱。
2. 将所有 `gateway.example.com` 改成你的真实域名。
3. 保留 `127.0.0.1:8080`，不要改成公网地址。

保存：按 `Ctrl+O`，回车确认；退出：按 `Ctrl+X`。然后回到 Linux
终端执行：

~~~bash
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
cd /opt/ai-token/current
/usr/local/go/bin/go run ./cmd/bootstrap-admin
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

更新前先把 `YOUR_RELEASE_TAG` 替换成新版本的真实 Git 标签：

~~~bash
export RELEASE=YOUR_RELEASE_TAG
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
