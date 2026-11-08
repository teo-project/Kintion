#!/usr/bin/env bash
# Проверка бэкапа целиком (T26): отдельная установка → данные (запись, фото,
# журнал) → backup.sh → down -v → restore.sh → данные на месте. В конце
# установка и её тома удаляются.
#
#   deploy/scripts/backup-check.sh        # порт KIN_CHECK_PORT, по умолчанию 18080
#   KIN_CHECK_KEEP=1 — не удалять установку и папку бэкапа (разбор ошибки)
. "$(dirname "$0")/lib.sh"

PORT=${KIN_CHECK_PORT:-18080}
export TEO_KIN_HTTP_BIND=$PORT
export KIN_COMPOSE="docker compose -p kintion-backupcheck -f \"$KIN_DIR/compose.yml\""
URL="http://localhost:$PORT"
WORK=$(cd "$(mktemp -d)" && { pwd -W 2>/dev/null || pwd; })
JAR="$WORK/cookies"
PASS="check-$(date +%s)-password"

cleanup() {
  [ -z "${KIN_CHECK_KEEP:-}" ] || { log "оставлено: $WORK, проект kintion-backupcheck"; return; }
  compose down -v >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

fail() {
  log "ОШИБКА: $*"
  exit 1
}

# api МЕТОД ПУТЬ [JSON] — запрос с cookie сессии и CSRF-токеном
api() {
  curl -fsS -b "$JAR" -c "$JAR" -X "$1" -H "X-Kintion-CSRF: ${CSRF:-}" \
    -H 'Content-Type: application/json' ${3:+--data "$3"} "$URL$2"
}

field() { # field ИМЯ — первое строковое значение "ИМЯ":"…" из stdin
  grep -o "\"$1\":\"[^\"]*\"" | head -n1 | cut -d'"' -f4
}

wait_healthy() {
  for _ in $(seq 60); do
    curl -fsS "$URL/healthz" >/dev/null 2>&1 && return 0
    sleep 2
  done
  fail "приложение не отвечает на $URL/healthz"
}

log "1. новая установка на порту $PORT"
compose up -d --build --wait
wait_healthy
TOKEN=$(compose logs app 2>/dev/null | field bootstrap_token)
[ -n "$TOKEN" ] || fail "нет токена настройки в логе"
CSRF=$(api POST /api/setup "{\"token\":\"$TOKEN\",\"login\":\"owner\",\"password\":\"$PASS\"}" | field csrf)
D=$(api GET /api/dashboards | field id)

log "2. данные: запись, фото, журнал"
api POST "/api/dashboards/$D/operations" \
  '{"records":[{"ref":"p","template":"kin.core.person","fields":{"name":[{"value":"Backup check"}]}}],"participants":[]}' >/dev/null
# PNG 1×1
printf '%s' 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==' | base64 -d >"$WORK/p.png"
SHA=$(curl -fsS -b "$JAR" -H "X-Kintion-CSRF: $CSRF" -F "file=@$WORK/p.png;type=image/png" "$URL/api/dashboards/$D/files" | field sha256)
[ -n "$SHA" ] || fail "фото не загрузилось"
OPS=$(api GET "/api/dashboards/$D/audit?limit=200" | grep -o '"operation_id"' | wc -l)

log "3. бэкап"
"$(dirname "$0")/backup.sh" "$WORK/backup"

log "4. down -v: установки больше нет"
compose down -v
log "5. восстановление"
"$(dirname "$0")/restore.sh" "$WORK/backup"
wait_healthy

log "6. проверка"
rm -f "$JAR"
CSRF=$(api POST /api/login "{\"login\":\"owner\",\"password\":\"$PASS\"}" | field csrf)
[ -n "$CSRF" ] || fail "вход после восстановления"
api GET "/api/dashboards/$D/records" | grep "Backup check" >/dev/null || fail "нет записи"
curl -fsS -b "$JAR" -o "$WORK/p2.png" "$URL/api/files/$D/$SHA" && cmp -s "$WORK/p.png" "$WORK/p2.png" || fail "нет фото"
OPS2=$(api GET "/api/dashboards/$D/audit?limit=200" | grep -o '"operation_id"' | wc -l)
[ "$OPS2" -ge "$OPS" ] || fail "журнал: было $OPS операций, стало $OPS2"
log "OK: запись, фото и журнал ($OPS2 операций) на месте"
