# Общее для backup.sh, restore.sh и backup-check.sh. Подключается через «.».
#
# KIN_COMPOSE — команда Compose установки (по умолчанию — deploy/compose.yml
# рядом со скриптом); другой проект: KIN_COMPOSE="docker compose -p имя -f …".
# KIN_TAR_IMAGE — образ с tar для тома файлов (образ приложения — distroless,
# без оболочки); по умолчанию — образ встроенного PostgreSQL, он уже скачан.

set -eu
# Git Bash на Windows: не переписывать пути вида /data в аргументах docker
export MSYS_NO_PATHCONV=1

# pwd -W — путь Windows в Git Bash (пути не переписываются, см. выше)
KIN_DIR=$(cd "$(dirname "$0")/.." && { pwd -W 2>/dev/null || pwd; })
KIN_COMPOSE=${KIN_COMPOSE:-docker compose -f "$KIN_DIR/compose.yml"}
KIN_TAR_IMAGE=${KIN_TAR_IMAGE:-postgres:18-alpine}
DB_USER=kintion
DB_NAME=kintion

compose() {
  eval "$KIN_COMPOSE" '"$@"'
}

log() {
  printf '%s\n' "$*" >&2
}

# id контейнера приложения (создаётся без запуска, если его ещё нет):
# через него подключается том файлов /data.
app_container() {
  id=$(compose ps -a -q app)
  if [ -z "$id" ]; then
    compose create app >/dev/null 2>&1
    id=$(compose ps -a -q app)
  fi
  [ -n "$id" ] || { log "нет контейнера app"; exit 1; }
  printf '%s' "$id"
}
