#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${AI_TOKEN_ENV_FILE:-/etc/ai-token/ai-token.env}"

if [[ ! -r "$ENV_FILE" ]]; then
  echo "AI Token environment file is unreadable: $ENV_FILE" >&2
  exit 1
fi

set -a
source "$ENV_FILE"
set +a

exec /opt/ai-token/current/bin/ai-token
