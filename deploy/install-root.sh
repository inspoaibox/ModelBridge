#!/usr/bin/env bash
set -Eeuo pipefail

REPO_URL="https://github.com/inspoaibox/ModelBridge.git"
BRANCH="main"
DOMAIN=""
ADMIN_EMAIL=""

usage() {
  cat <<'EOF'
Usage:
  bash deploy/install-root.sh --domain api.example.com [--admin-email admin@example.com]

This installer must run as root on a fresh Ubuntu or Debian server.
It installs AI Token with root-managed PM2, PostgreSQL, Caddy, and the first platform admin.
EOF
}

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain)
      DOMAIN="${2:-}"
      shift 2
      ;;
    --admin-email)
      ADMIN_EMAIL="${2:-}"
      shift 2
      ;;
    --repo)
      REPO_URL="${2:-}"
      shift 2
      ;;
    --branch)
      BRANCH="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "Unknown option: $1"
      ;;
  esac
done

[[ "${EUID}" -eq 0 ]] || fail "Run this installer as root."
[[ -n "$DOMAIN" ]] || fail "Pass your domain with --domain, for example: --domain api.example.com"
[[ "$DOMAIN" =~ ^[A-Za-z0-9.-]+$ ]] || fail "The domain contains unsupported characters."

if [[ -z "$ADMIN_EMAIL" ]]; then
  read -rp "Administrator email: " ADMIN_EMAIL
fi
[[ "$ADMIN_EMAIL" == *"@"* ]] || fail "Enter a valid administrator email."

read -rsp "Administrator password: " ADMIN_PASSWORD
echo
read -rsp "Repeat administrator password: " ADMIN_PASSWORD_CONFIRM
echo
[[ "$ADMIN_PASSWORD" == "$ADMIN_PASSWORD_CONFIRM" ]] || fail "Passwords do not match."
unset ADMIN_PASSWORD_CONFIRM

ENV_FILE="/etc/ai-token/ai-token.env"
RELEASE="$(date -u +%Y%m%d%H%M%S)"
RELEASE_DIR="/opt/ai-token/releases/$RELEASE"
PM2_HOME="/opt/ai-token/.pm2"
CADDYFILE="/etc/caddy/Caddyfile"

[[ ! -e "$ENV_FILE" ]] || fail "$ENV_FILE already exists. Do not overwrite an existing deployment."
[[ ! -e "$RELEASE_DIR" ]] || fail "Release directory already exists: $RELEASE_DIR"

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y \
  ca-certificates curl gnupg git \
  build-essential \
  postgresql postgresql-contrib
systemctl enable --now postgresql

if ! command -v go >/dev/null 2>&1 || ! go version | grep -q "go1.26.6"; then
  cd /tmp
  curl -fLO https://go.dev/dl/go1.26.6.linux-amd64.tar.gz
  curl -fLO https://go.dev/dl/go1.26.6.linux-amd64.tar.gz.sha256
  sha256sum -c go1.26.6.linux-amd64.tar.gz.sha256
  rm -rf /usr/local/go
  tar -C /usr/local -xzf go1.26.6.linux-amd64.tar.gz
fi
export PATH="/usr/local/go/bin:$PATH"

if ! command -v node >/dev/null 2>&1 || [[ "$(node -p 'process.versions.node.split(".")[0]')" -lt 22 ]]; then
  curl -fsSL https://deb.nodesource.com/setup_22.x -o /tmp/nodesource-22.sh
  bash /tmp/nodesource-22.sh
  apt-get install -y nodejs
fi
npm install --global pm2

if ! command -v caddy >/dev/null 2>&1; then
  apt-get install -y debian-keyring debian-archive-keyring apt-transport-https
  curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/gpg.key \
    | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
  curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt \
    | tee /etc/apt/sources.list.d/caddy-stable.list >/dev/null
  apt-get update
  apt-get install -y caddy
fi

install -d -m 0755 /opt/ai-token/releases
install -d -m 0700 "$PM2_HOME"
install -d -m 0700 /etc/ai-token
install -d -o caddy -g caddy -m 0750 /var/log/caddy

DB_PASSWORD="$(openssl rand -hex 24)"
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

if ! runuser -u postgres -- psql -tAc "SELECT 1 FROM pg_database WHERE datname = 'ai_token'" | grep -qx "1"; then
  runuser -u postgres -- createdb --owner=ai_token --encoding=UTF8 ai_token
fi
runuser -u postgres -- psql -d ai_token -v ON_ERROR_STOP=1 \
  -c "REVOKE ALL ON DATABASE ai_token FROM PUBLIC;"

git clone --depth 1 --branch "$BRANCH" "$REPO_URL" "$RELEASE_DIR"
ln -sfn "$RELEASE_DIR" /opt/ai-token/current

cd /opt/ai-token/current
go mod download
cd web
npm ci
npm run build
cd ..
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/ai-token ./cmd/server
test -x bin/ai-token
test -f web/dist/index.html
chmod 0755 deploy/pm2/start.sh

TOKEN_PEPPER="$(openssl rand -hex 48)"
SESSION_PEPPER="$(openssl rand -hex 48)"
MFA_ENCRYPTION_KEY="$(openssl rand -hex 32)"
umask 077
cat > "$ENV_FILE" <<EOF
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
REGISTRATION_ENABLED=false
REGISTRATION_EMAIL_VERIFICATION_REQUIRED=true
EOF
chown root:root "$ENV_FILE"
chmod 0600 "$ENV_FILE"

[[ -f "$CADDYFILE" ]] || fail "Caddy configuration not found at $CADDYFILE"
if grep -qE "^[[:space:]]*${DOMAIN//./\\.}[[:space:]]*\\{" "$CADDYFILE"; then
  fail "A Caddy site for $DOMAIN already exists. Add the reverse proxy manually instead."
fi
cat >> "$CADDYFILE" <<EOF

$DOMAIN {
	encode zstd gzip

	log {
		output file /var/log/caddy/ai-token.access.log {
			roll_size 100MiB
			roll_keep 10
			roll_keep_for 720h
		}
		format json
	}

	header {
		Strict-Transport-Security "max-age=31536000; includeSubDomains"
	}

	request_body {
		max_size 50MB
	}

	@relay path /v1/*
	reverse_proxy @relay 127.0.0.1:8080 {
		flush_interval -1
		header_up X-Forwarded-For {remote_host}
		header_up X-Real-IP {remote_host}
		header_up X-Forwarded-Proto {scheme}
	}

	reverse_proxy 127.0.0.1:8080 {
		header_up X-Forwarded-For {remote_host}
		header_up X-Real-IP {remote_host}
		header_up X-Forwarded-Proto {scheme}
	}
}
EOF

caddy validate --config "$CADDYFILE" --adapter caddyfile
systemctl enable --now caddy
systemctl reload caddy

install -m 0644 deploy/pm2/ai-token-pm2.service /etc/systemd/system/ai-token-pm2.service
systemctl daemon-reload
PM2_HOME="$PM2_HOME" pm2 startOrReload deploy/pm2/ecosystem.config.cjs --update-env
PM2_HOME="$PM2_HOME" pm2 save
PM2_HOME="$PM2_HOME" pm2 kill
systemctl enable --now ai-token-pm2

set -a
source "$ENV_FILE"
set +a
export ADMIN_EMAIL ADMIN_PASSWORD
go run ./cmd/bootstrap-admin
unset ADMIN_PASSWORD DB_PASSWORD TOKEN_PEPPER SESSION_PEPPER MFA_ENCRYPTION_KEY

for _ in {1..30}; do
  if curl -fsS http://127.0.0.1:8080/healthz >/dev/null; then
    echo "Installation complete: https://$DOMAIN"
    exit 0
  fi
  sleep 1
done

systemctl status ai-token-pm2 --no-pager || true
journalctl -u ai-token-pm2 -n 100 --no-pager || true
fail "The service did not become healthy. Review the logs above."
