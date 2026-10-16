#!/usr/bin/env bash
# Восстановление Kintion из бэкапа backup.sh — в пустую
# или существующую установку: текущие база и файлы ЗАМЕНЯЮТСЯ бэкапом.
#
#   deploy/scripts/restore.sh <папка бэкапа>
#
# Порядок: приложение останавливается → база пересоздаётся и заполняется из
# db.dump → том файлов очищается и заполняется из files.tar.gz → приложение
# запускается (миграции новее бэкапа применятся при старте).
. "$(dirname "$0")/lib.sh"

IN=${1:?укажите папку бэкапа}
[ -f "$IN/db.dump" ] && [ -f "$IN/files.tar.gz" ] || { log "в $IN нет db.dump и files.tar.gz"; exit 1; }

log "запуск базы"
compose up -d --wait db
APP=$(app_container)
compose stop app >/dev/null 2>&1 || true

log "база ← $IN/db.dump"
compose exec -T db dropdb -U "$DB_USER" --if-exists --force "$DB_NAME"
compose exec -T db createdb -U "$DB_USER" "$DB_NAME"
compose exec -T db pg_restore -U "$DB_USER" -d "$DB_NAME" --no-owner --exit-on-error <"$IN/db.dump"

log "файлы ← $IN/files.tar.gz"
docker run --rm -i --volumes-from "$APP" --entrypoint sh "$KIN_TAR_IMAGE" \
  -c 'find /data -mindepth 1 -delete && tar -xzf - -C /data --numeric-owner' <"$IN/files.tar.gz"

log "запуск приложения"
compose up -d app
log "готово"
