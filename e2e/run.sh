#!/usr/bin/env bash
# Сквозной тест 0.1 на свежей установке: compose up → Playwright → down -v.
#   e2e/run.sh [аргументы playwright test]
# KIN_E2E_PORT — порт (по умолчанию 18080); KIN_E2E_KEEP=1 — не удалять установку.
set -eu
cd "$(dirname "$0")"
export KIN_E2E_PORT=${KIN_E2E_PORT:-18080}
C="docker compose -f ../deploy/compose.yml -f ../deploy/compose.e2e.yml"

cleanup() {
  [ -n "${KIN_E2E_KEEP:-}" ] || $C down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

$C down -v >/dev/null 2>&1 || true
$C up -d --build --wait
# токен настройки — из лога первого запуска (F-28)
KIN_E2E_TOKEN=$($C logs app 2>/dev/null | grep -o '"bootstrap_token":"[^"]*"' | head -n1 | cut -d'"' -f4)
export KIN_E2E_TOKEN
[ -n "$KIN_E2E_TOKEN" ] || { echo "нет токена настройки в логе" >&2; exit 1; }
[ -d node_modules ] || npm ci
npx playwright test "$@"
