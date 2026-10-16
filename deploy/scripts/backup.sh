#!/usr/bin/env bash
# Бэкап Kintion: база — pg_dump -Fc, файлы — архив тома
# /data. Приложение не останавливается: дамп согласован сам по себе, файлы
# только добавляются (имя = хеш), так что архив, снятый после дампа, содержит
# всё, на что дамп ссылается.
#
#   deploy/scripts/backup.sh [папка]     # по умолчанию backups/<дата-время>
#
# Результат: db.dump, files.tar.gz, info.txt.
. "$(dirname "$0")/lib.sh"

OUT=${1:-backups/$(date +%Y%m%d-%H%M%S)}
mkdir -p "$OUT"

log "база → $OUT/db.dump"
compose exec -T db pg_dump -U "$DB_USER" -d "$DB_NAME" -Fc >"$OUT/db.dump"

log "файлы → $OUT/files.tar.gz"
docker run --rm --volumes-from "$(app_container)" --entrypoint tar "$KIN_TAR_IMAGE" \
  -czf - -C /data . >"$OUT/files.tar.gz"

{
  echo "created_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "schema=$(compose exec -T db psql -U "$DB_USER" -d "$DB_NAME" -tAc 'SELECT version FROM schema_info' 2>/dev/null || echo '?')"
  echo "app=$(compose exec -T db psql -U "$DB_USER" -d "$DB_NAME" -tAc 'SELECT app_version FROM schema_info' 2>/dev/null || echo '?')"
} >"$OUT/info.txt"

log "готово: $OUT"
