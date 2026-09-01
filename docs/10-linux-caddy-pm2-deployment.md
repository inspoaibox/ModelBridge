# Linux 部署手册：Caddy + PM2 + PostgreSQL

这是 AI Token Gateway 唯一维护的 Linux 部署流程，适用于 Debian 11/12、Ubuntu
22.04/24.04 的 x86_64 服务器。

部署、构建和 PM2 均使用 `root`。不会创建 `ai-token` Linux 用户；只有 PostgreSQL
自带的 `postgres` 数据库管理员账号会用于创建业务数据库。

> **先读这一段**
>
> 1. 本文没有一键安装脚本，也没有 `--resume`。
> 2. 从第 1 步开始，按顺序执行；不要跳到后面的构建、PM2 或 Caddy 步骤。
> 3. 所有 `bash` 代码块都直接粘贴到 `root@服务器:~#` 终端执行。
> 4. `caddyfile` 代码块不是终端命令，是要追加到 `/etc/caddy/Caddyfile` 的配置内容。
> 5. 若服务器已经有同一个域名的 Caddy 站点，立刻停止本文流程，不要追加第二个同域名站点块，也不要覆盖现有 `Caddyfile`。

本文示例域名为 `gateway.example.com`。在第 1 步替换成自己的真实域名。

## 0. 当前已被旧安装器中断的服务器

如果你曾经运行过旧的 `install-root.sh`，并看到：

```text
ERROR: /etc/ai-token/ai-token.env already exists
```

这表示旧脚本已经写入过环境文件。**不要再运行旧脚本，不要删除环境文件，不要重新创建数据库。**

先执行下面的只读检查：

```bash
test -f /etc/ai-token/ai-token.env && echo "环境文件：存在"
test -d /opt/ai-token/current && echo "当前发布目录：存在"
test -x /opt/ai-token/current/bin/ai-token && echo "后端二进制：存在"
test -f /opt/ai-token/current/web/dist/index.html && echo "前端构建：存在"
runuser -u postgres -- psql -d ai_token -c 'SELECT current_database(), current_user;'
```

若五项都正常，旧脚本已经完成了数据库、构建和环境文件；当前只需要处理 **Caddy
同域名配置**、PM2 和管理员初始化。跳到第 7 节前，先执行下面的命令，并把输出中的
`gateway.aokede.com` 站点块完整提供给维护人员：

```bash
grep -n -B 4 -A 80 -F 'gateway.aokede.com' /etc/caddy/Caddyfile
```

这条命令只读取 Caddy 配置，不会修改任何内容。Caddyfile 不应包含数据库密码、上游
API Key 或用户 Token；如你的文件确实包含此类值，先删掉这些行再提供。

如果上述检查中有任意一项不存在，不要混用旧脚本和本文步骤，也不要直接删除现有目录、
数据库或环境文件。先执行下面的只读命令并保留输出，确认旧脚本已经做到哪一步后，再从
缺失的那一节继续：

```bash
ls -ld /etc/ai-token /opt/ai-token /opt/ai-token/current /opt/ai-token/releases 2>&1
find /opt/ai-token -maxdepth 2 -type f \( -name ai-token -o -name ai-token.env \) -print 2>/dev/null
```

## 1. 确认服务器、域名和端口

登录服务器后确认当前就是 root，并设置本次部署使用的域名。不要关闭这个终端；后续
代码块会继续使用 `DOMAIN` 变量。

```bash
id -un
uname -m
export DOMAIN=gateway.example.com
printf '部署域名：%s\n' "$DOMAIN"
```

预期第一行输出 `root`，第二行输出 `x86_64`。如果不是 `x86_64`，不要继续使用本文
的 Go 下载地址，应改为对应 CPU 架构的官方 Go 安装包。

开始前确认：

1. 域名的 A/AAAA 记录已经指向本服务器公网 IP。
2. 云服务器安全组和本机防火墙允许 TCP `80`、`443` 和 SSH。
3. 不对公网开放 `8080`、`5432`。
4. 如已有 Nginx、Apache 或 Caddy，先确认哪个服务正在管理 `80` 和 `443`，不能让多个 Web 服务抢占端口。

以下命令仅用于查看端口占用：

```bash
ss -lntp | grep -E ':(80|443|8080|5432)\b' || true
```

## 2. 安装系统依赖

初次部署**不执行** `apt full-upgrade`，也**不要求重启服务器**。系统更新和内核重启是
服务器维护工作，应单独安排，不属于应用安装步骤。

```bash
apt-get update
apt-get install -y \
  ca-certificates curl gnupg git \
  build-essential \
  postgresql postgresql-contrib
systemctl enable --now postgresql
systemctl status postgresql --no-pager
```

## 3. 安装 Go、Node.js、PM2 和 Caddy

这一节只安装运行和构建所需软件，不会创建数据库、不写环境文件、不修改
`/etc/caddy/Caddyfile`。

### 3.1 Go

项目当前要求 Go `1.26.x`。下面命令下载 Go 官方发布包、校验 SHA-256 后安装到
`/usr/local/go`。

```bash
cd /tmp
curl -fLO https://go.dev/dl/go1.26.6.linux-amd64.tar.gz
GO_SHA256="$(curl -fsSL https://go.dev/dl/go1.26.6.linux-amd64.tar.gz.sha256)"
printf '%s  %s\n' "$GO_SHA256" go1.26.6.linux-amd64.tar.gz | sha256sum -c -
rm -rf /usr/local/go
tar -C /usr/local -xzf go1.26.6.linux-amd64.tar.gz
ln -sf /usr/local/go/bin/go /usr/local/bin/go
go version
```

最后一行应显示 `go version go1.26.6 ...`。如果校验失败，立即停止，不要继续安装。

### 3.2 Node.js 和 PM2

下面步骤显式写入 NodeSource 的 APT 软件源，不执行第三方“快速安装脚本”。

```bash
curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
  | gpg --dearmor --yes -o /usr/share/keyrings/nodesource.gpg
printf '%s\n' \
  'deb [signed-by=/usr/share/keyrings/nodesource.gpg] https://deb.nodesource.com/node_22.x nodistro main' \
  > /etc/apt/sources.list.d/nodesource.list
apt-get update
apt-get install -y nodejs
npm install -g pm2
node --version
npm --version
command -v pm2
```

`node --version` 应为 `v22` 或更高版本；最后一行会显示 PM2 命令所在路径。

### 3.3 Caddy

先安装 Caddy 软件包。此步骤不会改动现有的 `/etc/caddy/Caddyfile`。

```bash
apt-get install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/gpg.key \
  | gpg --dearmor --yes -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt \
  > /etc/apt/sources.list.d/caddy-stable.list
apt-get update
apt-get install -y caddy
caddy version
```

## 4. 先检查 Caddy 是否已占用域名

在下载应用、创建数据库和环境文件**之前**，先检查目标域名是否已出现在 Caddy 配置中：

```bash
grep -n -F "$DOMAIN" /etc/caddy/Caddyfile && {
  echo "停止：该域名已经在 Caddyfile 中配置。不要追加、不要覆盖。"
  exit 20
}
echo "未发现同域名 Caddy 站点，可以继续。"
```

若输出“停止”，说明这个域名已经被其他 Caddy 站点使用。可能是旧项目、已有网站或已
存在的 AI Token 站点。此时不可以猜测如何合并配置；先执行：

```bash
grep -n -B 4 -A 80 -F "$DOMAIN" /etc/caddy/Caddyfile
```

确认该域名现有站点的用途后再编辑那一个站点块。**绝不新增第二个同域名站点块。**

若输出“未发现同域名 Caddy 站点”，继续下一节。

## 5. 下载并构建应用

以下命令下载当前 `main` 分支。`RELEASE` 只是服务器上的发布目录名，命令会自动使用
当前 UTC 时间生成它；不需要手动填写日期或 Git 标签。

```bash
export REPO_URL=https://github.com/inspoaibox/ModelBridge.git
export RELEASE="$(date -u +%Y%m%d-%H%M%S)"
export RELEASE_DIR="/opt/ai-token/releases/$RELEASE"

install -d -m 0755 /opt/ai-token/releases
git clone --depth 1 --branch main "$REPO_URL" "$RELEASE_DIR"
ln -sfn "$RELEASE_DIR" /opt/ai-token/current

cd /opt/ai-token/current
go mod download
cd web
npm ci
npm run build
cd ..
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/ai-token ./cmd/server
chmod 0755 deploy/pm2/start.sh

test -x /opt/ai-token/current/bin/ai-token
test -f /opt/ai-token/current/web/dist/index.html
git rev-parse --short HEAD
```

这一节只做编译，不在用户服务器上运行单元测试、竞态测试、漏洞扫描或 `npm audit`。
这些应由 CI 或发布前验证环境完成。最后一行显示本次运行的 Git 提交短号，记录下来
即可。

如果此处出现 `cd: /opt/ai-token/current: No such file or directory`，说明前面的
`git clone` 没有成功或没有执行；回到本节最开始重新执行，而不是单独复制后面的
`go mod download`。

## 6. 创建数据库和生产环境文件

这一步会：

1. 创建 PostgreSQL 业务账号 `ai_token` 和数据库 `ai_token`。
2. 生成数据库密码、Token Pepper、Session Pepper 和 MFA 加密密钥。
3. 只把这些值写入 root 可读的 `/etc/ai-token/ai-token.env`。

命令不会在终端输出任何生成的密码或密钥。不要在已上线且已有真实用户的数据环境重复
执行本节，因为重新生成 Pepper 会使现有 Session 和 API Token 失效。

```bash
set -e
install -d -m 0700 /etc/ai-token

DB_PASSWORD="$(openssl rand -hex 24)"
TOKEN_PEPPER="$(openssl rand -hex 48)"
SESSION_PEPPER="$(openssl rand -hex 48)"
MFA_ENCRYPTION_KEY="$(openssl rand -hex 32)"
ADMIN_ENTRY_SUFFIX="$(openssl rand -hex 16)"

runuser -u postgres -- psql -v ON_ERROR_STOP=1 <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ai_token') THEN
    CREATE ROLE ai_token LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
  END IF;
END
\$\$;
SET password_encryption = 'scram-sha-256';
ALTER ROLE ai_token PASSWORD '$DB_PASSWORD';
SQL

if ! runuser -u postgres -- psql -tAc "SELECT 1 FROM pg_database WHERE datname = 'ai_token'" | grep -qx '1'; then
  runuser -u postgres -- createdb --owner=ai_token --encoding=UTF8 ai_token
fi
runuser -u postgres -- psql -d ai_token -v ON_ERROR_STOP=1 \
  -c 'REVOKE ALL ON DATABASE ai_token FROM PUBLIC;'

umask 077
cat > /etc/ai-token/ai-token.env <<EOF
APP_ENV=production
HTTP_ADDR=127.0.0.1:8080
HTTP_READ_TIMEOUT=60s
HTTP_WRITE_TIMEOUT=15m
HTTP_IDLE_TIMEOUT=2m
WEB_DIR=/opt/ai-token/current/web
MIGRATIONS_DIR=/opt/ai-token/current/migrations
DATABASE_URL=postgres://ai_token:$DB_PASSWORD@127.0.0.1:5432/ai_token?sslmode=disable
TOKEN_PEPPER=$TOKEN_PEPPER
SESSION_PEPPER=$SESSION_PEPPER
MFA_ENCRYPTION_KEY=$MFA_ENCRYPTION_KEY
COOKIE_SECURE=true
SESSION_TTL=12h
LOGIN_MAX_FAILURES=5
LOGIN_FAILURE_WINDOW=15m
LOGIN_LOCK_DURATION=15m
CORS_ALLOWED_ORIGINS=https://$DOMAIN
TRUSTED_PROXY_CIDRS=127.0.0.1/32,::1/128
ADMIN_ENTRY_PATH=/admin-$ADMIN_ENTRY_SUFFIX
REGISTRATION_ENABLED=true
REGISTRATION_EMAIL_VERIFICATION_REQUIRED=true
EOF

chown root:root /etc/ai-token/ai-token.env
chmod 0600 /etc/ai-token/ai-token.env
printf '请保存管理员入口：https://%s/admin-%s\n' "$DOMAIN" "$ADMIN_ENTRY_SUFFIX"
unset DB_PASSWORD TOKEN_PEPPER SESSION_PEPPER MFA_ENCRYPTION_KEY ADMIN_ENTRY_SUFFIX

stat -c '环境文件权限：%a %U:%G' /etc/ai-token/ai-token.env
```

最后一行必须显示：

```text
环境文件权限：600 root:root
```

`REGISTRATION_ENABLED=true` 使前端用户可以自助注册。邮件功能总开关默认关闭，因此在
管理员后台完成 SMTP 配置并打开邮件与邮箱验证功能前，新注册账户会直接为可用状态；
邮件系统本身不写入环境文件。

验证 PostgreSQL 可通过 TCP 与业务账号连接：

```bash
set -a
source /etc/ai-token/ai-token.env
set +a
psql "$DATABASE_URL" -c 'SELECT current_database(), current_user;'
unset DATABASE_URL TOKEN_PEPPER SESSION_PEPPER MFA_ENCRYPTION_KEY
```

如果这一步报 PostgreSQL 认证或连接错误，不要把 `listen_addresses = ...` 直接粘贴到
Shell，也不要随意修改 `postgresql.conf`。先收集下面两条输出后再处理：

```bash
runuser -u postgres -- psql -tAc 'SHOW config_file'
runuser -u postgres -- psql -tAc 'SHOW hba_file'
```

## 7. 启动 PM2 和应用

应用第一次启动会自动执行数据库迁移。PM2 状态和日志只放在
`/opt/ai-token/.pm2`，不会覆盖服务器上的其他 PM2 项目。

```bash
install -d -m 0700 /opt/ai-token/.pm2
install -m 0644 \
  /opt/ai-token/current/deploy/pm2/ai-token-pm2.service \
  /etc/systemd/system/ai-token-pm2.service

systemctl daemon-reload
PM2_HOME=/opt/ai-token/.pm2 pm2 startOrReload \
  /opt/ai-token/current/deploy/pm2/ecosystem.config.cjs --update-env
PM2_HOME=/opt/ai-token/.pm2 pm2 save
PM2_HOME=/opt/ai-token/.pm2 pm2 kill
systemctl enable --now ai-token-pm2

systemctl status ai-token-pm2 --no-pager
curl -fsS http://127.0.0.1:8080/healthz
```

最后一条应返回健康响应。如果失败，先看应用日志：

```bash
PM2_HOME=/opt/ai-token/.pm2 pm2 logs ai-token --lines 100
journalctl -u ai-token-pm2 -n 100 --no-pager
```

在本机健康检查未通过前，不要进入 Caddy 配置步骤。

## 8. 追加 Caddy 站点配置

仅当第 4 节已经确认 **没有** 同域名站点，并且第 7 节本机健康检查通过时，才执行此节。

打开现有 Caddy 主配置文件：

```bash
nano /etc/caddy/Caddyfile
```

移动到文件最末尾，追加下面整个站点块。将 `gateway.example.com` 替换为真实域名。
不要加入 `email` 行；Caddy 自动 HTTPS 不要求在此处配置通知邮箱。

```caddyfile
gateway.example.com {
	encode zstd gzip

	request_body {
		max_size 50MB
	}

	@relay path /v1/*
	reverse_proxy @relay 127.0.0.1:8080 {
		flush_interval -1
	}

	reverse_proxy 127.0.0.1:8080
}
```

在 nano 中按 `Ctrl+O` 保存，回车确认文件名；按 `Ctrl+X` 退出。随后校验并加载：

```bash
caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
systemctl enable --now caddy
systemctl reload caddy
systemctl status caddy --no-pager
curl -fsS "https://$DOMAIN/healthz"
```

`caddy validate` 出错时，不要 reload；先根据报错修复刚追加的站点块。不要覆盖其他
站点，也不要在已有同域名的情况下追加此块。

## 9. 创建首个管理员

管理员不是通过公开注册创建。下面命令会在终端询问管理员邮箱和密码，密码输入时不会
显示字符；首次执行还会输出一次 TOTP Secret，必须立即保存到认证器。

```bash
set -a
source /etc/ai-token/ai-token.env
set +a
read -rp '管理员邮箱：' ADMIN_EMAIL
read -rsp '管理员密码：' ADMIN_PASSWORD
echo
export ADMIN_EMAIL ADMIN_PASSWORD
cd /opt/ai-token/current
go run ./cmd/bootstrap-admin
unset ADMIN_EMAIL ADMIN_PASSWORD DATABASE_URL TOKEN_PEPPER SESSION_PEPPER MFA_ENCRYPTION_KEY
```

然后访问：

```text
https://你的域名
```

用刚创建的管理员账号登录。SMTP、发件人、模板、邮箱验证、密码重置和邮件总开关均在
“系统设置 → 邮件设置 / 功能开关”中配置；不需要，也不应该把 SMTP 密码写入
`/etc/ai-token/ai-token.env`。

管理员入口由环境文件中的 `ADMIN_ENTRY_PATH` 控制，例如
`https://你的域名/admin-随机后缀`。它不是前台菜单的一部分；只有访问这个路径后才可
以提交管理员登录。将完整地址保存到密码管理器，勿发布到公开文档、客服页面或前端菜单。

## 10. 上线后最小验收

```bash
curl -fsS "https://$DOMAIN/healthz"
curl -I "https://$DOMAIN/"
PM2_HOME=/opt/ai-token/.pm2 pm2 status
runuser -u postgres -- psql -d ai_token -c \
  'SELECT version, applied_at FROM schema_migrations ORDER BY version DESC LIMIT 5;'
```

完成浏览器验收：

1. 首页、登录、注册和模型广场可以正常打开。
2. 管理员可进入渠道、分组、价格、用户、财务、使用记录和系统设置。
3. 新注册用户可以进入控制台、创建 API Token、选择分组并查看账务和使用记录。
4. 配置 SMTP 后再开启邮件总开关和邮箱验证，并发送测试邮件。
5. 使用真实上游 Key 进行一条受限 Token 的模型调用，确认用量和账务记录正确。

## 11. 日常查看、备份与更新

### 查看状态

```bash
systemctl status caddy ai-token-pm2 --no-pager
PM2_HOME=/opt/ai-token/.pm2 pm2 status
PM2_HOME=/opt/ai-token/.pm2 pm2 logs ai-token --lines 200
journalctl -u caddy -u ai-token-pm2 -n 200 --no-pager
```

### 备份数据库

```bash
install -d -o postgres -g postgres -m 0700 /var/backups/ai-token
runuser -u postgres -- pg_dump -Fc -d ai_token \
  -f "/var/backups/ai-token/ai-token-$(date +%F).dump"
```

备份文件中含有用户、渠道加密密文、账务和审计数据，必须加密保存且限制 root/
PostgreSQL 管理员以外的访问。

### 更新应用

更新前先完成数据库备份。以下操作只更新应用代码，不会重建数据库、不改环境文件、不改
Caddy 配置。

```bash
export REPO_URL=https://github.com/inspoaibox/ModelBridge.git
export RELEASE="$(date -u +%Y%m%d-%H%M%S)"
export RELEASE_DIR="/opt/ai-token/releases/$RELEASE"

git clone --depth 1 --branch main "$REPO_URL" "$RELEASE_DIR"
cd "$RELEASE_DIR"
go mod download
cd web
npm ci
npm run build
cd ..
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/ai-token ./cmd/server
chmod 0755 deploy/pm2/start.sh
test -x bin/ai-token
test -f web/dist/index.html

if ! grep -q '^ADMIN_ENTRY_PATH=' /etc/ai-token/ai-token.env; then
  ADMIN_ENTRY_SUFFIX="$(openssl rand -hex 16)"
  printf '\nADMIN_ENTRY_PATH=/admin-%s\n' "$ADMIN_ENTRY_SUFFIX" >> /etc/ai-token/ai-token.env
  chmod 0600 /etc/ai-token/ai-token.env
  printf '请保存管理员入口：https://%s/admin-%s\n' "$DOMAIN" "$ADMIN_ENTRY_SUFFIX"
  unset ADMIN_ENTRY_SUFFIX
fi

ln -sfn "$RELEASE_DIR" /opt/ai-token/current
PM2_HOME=/opt/ai-token/.pm2 pm2 startOrReload \
  /opt/ai-token/current/deploy/pm2/ecosystem.config.cjs --update-env
PM2_HOME=/opt/ai-token/.pm2 pm2 save
curl -fsS "https://$DOMAIN/healthz"
```

如果新版本启动失败，先切回上一次目录，再重载 PM2：

```bash
ln -sfn /opt/ai-token/releases/上一版本目录名 /opt/ai-token/current
PM2_HOME=/opt/ai-token/.pm2 pm2 startOrReload \
  /opt/ai-token/current/deploy/pm2/ecosystem.config.cjs --update-env
PM2_HOME=/opt/ai-token/.pm2 pm2 save
```

只有确认新旧数据库迁移兼容时才能只回滚应用。若迁移不兼容，必须停服务、恢复已验证的
数据库备份，再切回旧版本。

## 12. 常见问题

| 现象 | 原因与处理 |
| --- | --- |
| `/etc/ai-token/ai-token.env already exists` | 旧安装器留下的半安装状态。不要删除和重跑脚本，按第 0 节只读检查，再继续未完成的 Caddy、PM2 和管理员步骤。 |
| `cd /opt/ai-token/current: No such file or directory` | 下载或软链接步骤没有完成。回到第 5 节从 `git clone` 开始执行，不能只执行构建尾部命令。 |
| Caddy 提示同域名站点已存在 | 该域名已有业务配置。不要追加、不要覆盖；读取原站点块后再决定如何把反代加入其中。 |
| `caddy validate` 失败 | 只修改刚追加的 AI Token 站点块，修复后重新 validate；验证通过前不要 reload。 |
| 访问返回 502 | 先运行本机 `curl http://127.0.0.1:8080/healthz`，再看 PM2 和 systemd 日志。 |
| 登录或注册不可用 | 检查 PM2 日志、数据库迁移和 `REGISTRATION_ENABLED`；邮件总开关关闭时不依赖 SMTP。 |
| 密码重置邮件未发送 | 在管理员后台配置 SMTP，发送测试邮件，再开启邮件总开关和“密码重置”事件开关。 |

## 13. 不放在部署服务器执行的工作

以下是发布前质量验证，应该在 CI 或开发/预发布环境完成，而不是让初次部署用户在生产
服务器上等待数小时：

```text
go test ./...
go test -race ./...
go vet ./...
govulncheck ./...
npm run lint
npm run typecheck
npm test
npm audit
```

生产服务器只需要完成第 1 至第 10 节的安装、健康检查和真实业务验收。
