# Deployment

Needs Docker with Compose v2. The frontend and the server are built inside the image.

Files used below are in [`deploy/`](../deploy): `compose.yml` and `.env.example`.

## Start

```sh
cd deploy
cp .env.example .env            # set TEO_KIN_DB_PASSWORD before the first start
docker compose up -d --build
curl localhost:8080/healthz     # {"status":"ok","db":"ok"}
```

| Service | What | Volume |
| --- | --- | --- |
| `app` | Kintion: Go with the embedded Vue application, runs as non-root | `kintion-data` → `/data` (files) |
| `db` | PostgreSQL 18 | `kintion-db` |
| `backup`, `restore` | on demand only (profile `tools`) | see below |

**The database password is applied only when the volume is created.** Changing `TEO_KIN_DB_PASSWORD` later does not change the password stored in the volume. Either recreate the volumes (`docker compose down -v` — **deletes all data**) or change the password inside PostgreSQL (`ALTER USER kintion PASSWORD '…'`) and restart `app`.

## First start: the owner

There is no registration. The first account is created with a one-time **setup token** that the application writes to its log while no owner exists:

```sh
docker compose logs app | grep bootstrap_token
```

Open the interface, enter the token, a login and a password. A first dashboard is created and the token stops working. The owner is the only account and the administrator.

Forgotten password (there is no e-mail yet) — a command in the container sets a new random password and ends all sessions of the owner:

```sh
docker compose exec app /usr/local/bin/kintion reset-password --login <login>
```

## Address and port

`TEO_KIN_HTTP_BIND` accepts an address (`127.0.0.1:18080`, `[::1]:18080`, `:18080`) or just a port (`18080` = `:18080`). One address only; for several addresses or a unix socket use a reverse proxy.

Without Docker — `kintion` is the application binary itself (the same one that runs inside the image, built from `server/cmd`); the variable is set just for that one launch:

```sh
TEO_KIN_HTTP_BIND=18080 ./kintion                # all interfaces, IPv4 and IPv6
TEO_KIN_HTTP_BIND=127.0.0.1:18080 ./kintion      # this machine only
TEO_KIN_HTTP_BIND='[::1]:18080' ./kintion        # IPv6 loopback
TEO_KIN_HTTP_BIND=192.168.1.10:18080 ./kintion   # one LAN address
```

In Compose the variable stands on both sides of `ports` and is passed into the container, so it must be **a port only**. Restrict the *host* interface in `ports`:

```yaml
    ports:
      - "127.0.0.1:${TEO_KIN_HTTP_BIND:-8080}:${TEO_KIN_HTTP_BIND:-8080}"   # this machine only (behind a reverse proxy)
```

If a reverse proxy runs in the same Docker network you can omit `ports` and let the proxy reach `app:<port>`.

## Dockge / Portainer (without cloning)

No repository clone is needed: Docker builds the image straight from a tagged git revision (`context: "https://github.com/teo-project/Kintion.git#v0.1.0"`, `dockerfile: deploy/Dockerfile`). Paste the file below as a new stack in Dockge (or into the Portainer stack editor, "Web editor").

```yaml
# Kintion for Dockge / Portainer: no repository clone needed, the image is
# built from the tagged git revision. To update: change the tag in "context"
# and redeploy with a rebuild.
name: kintion

services:
  app:
    build:
      context: "https://github.com/teo-project/Kintion.git#v0.1.0"
      dockerfile: deploy/Dockerfile
      args:
        VERSION: ${TEO_KIN_VERSION:-v0.1.0}
    image: kintion:${TEO_KIN_VERSION:-v0.1.0}
    restart: unless-stopped
    environment:
      # external database: set TEO_KIN_DATABASE_URL in .env
      TEO_KIN_DATABASE_URL: ${TEO_KIN_DATABASE_URL:-postgres://kintion:${TEO_KIN_DB_PASSWORD:-kintion}@db:5432/kintion?sslmode=disable}
      TEO_KIN_PUBLIC_URL: ${TEO_KIN_PUBLIC_URL:-}
      TEO_KIN_LOG_LEVEL: ${TEO_KIN_LOG_LEVEL:-info}
      TEO_KIN_LANGUAGE: ${TEO_KIN_LANGUAGE:-en}
      TEO_KIN_PACKS: ${TEO_KIN_PACKS:-kin.core}
      TEO_KIN_AUTH_MODE: ${TEO_KIN_AUTH_MODE:-local}
      TEO_KIN_REGISTRATION: ${TEO_KIN_REGISTRATION:-closed}
      TEO_KIN_UPLOAD_MAX_MB: ${TEO_KIN_UPLOAD_MAX_MB:-20}
      # in Compose this is a port only: it is used on both sides of "ports"
      TEO_KIN_HTTP_BIND: ${TEO_KIN_HTTP_BIND:-8080}
    ports:
      - "${TEO_KIN_HTTP_BIND:-8080}:${TEO_KIN_HTTP_BIND:-8080}"
    volumes:
      - kintion-data:/data
    depends_on:
      db:
        condition: service_healthy

  db:
    image: postgres:18-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: kintion
      POSTGRES_PASSWORD: ${TEO_KIN_DB_PASSWORD:-kintion}
      POSTGRES_DB: kintion
    # the port is not published: only the application reaches the database
    volumes:
      - kintion-db:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U kintion -d kintion"]
      interval: 5s
      timeout: 3s
      retries: 20

  # One-command backup:  docker compose run --rm backup
  # Result: volume kintion-backups, folder <date-time>-<n>/ with db.dump, files.tar.gz and info.txt
  # (info.txt is written last; while a backup runs the folder ends with .partial).
  backup:
    profiles: ["tools"]
    image: postgres:18-alpine
    depends_on:
      db:
        condition: service_healthy
    environment: &pg
      PGHOST: db
      PGUSER: kintion
      PGDATABASE: kintion
      PGPASSWORD: ${TEO_KIN_DB_PASSWORD:-kintion}
      # with an external database these services refuse to run
      TEO_KIN_DATABASE_URL: ${TEO_KIN_DATABASE_URL:-}
    volumes:
      - kintion-data:/data:ro
      - kintion-backups:/backups
    entrypoint: ["sh", "-eu", "-c"]
    command:
      - |
        [ -z "$$TEO_KIN_DATABASE_URL" ] || { echo "external database (TEO_KIN_DATABASE_URL): dump it with your own tools, see docs/deployment.md"; exit 1; }
        final=/backups/$$(date +%Y%m%d-%H%M%S)-$$$$
        out="$$final.partial"
        mkdir -p "$$out"
        pg_dump -Fc > "$$out/db.dump"
        tar -czf "$$out/files.tar.gz" -C /data .
        {
          echo "created_at=$$(date -u +%Y-%m-%dT%H:%M:%SZ)"
          echo "schema=$$(psql -tAc 'SELECT version FROM schema_info' || echo '?')"
          echo "app=$$(psql -tAc 'SELECT app_version FROM schema_info' || echo '?')"
        } > "$$out/info.txt"
        mv "$$out" "$$final"
        echo "done: $$final"

  # Restore REPLACES the database and the files with a backup.
  # Stop "app" first (keep "db" running), then:
  #   docker compose run --rm restore                      # latest finished backup
  #   RESTORE_FROM=<folder> docker compose run --rm restore
  # and start "app" again. A running application makes it refuse
  # (RESTORE_FORCE=1 skips the check).
  restore:
    profiles: ["tools"]
    image: postgres:18-alpine
    depends_on:
      db:
        condition: service_healthy
    environment:
      <<: *pg
      RESTORE_FROM: ${RESTORE_FROM:-}
      RESTORE_FORCE: ${RESTORE_FORCE:-}
    volumes:
      - kintion-data:/data
      - kintion-backups:/backups:ro
    entrypoint: ["sh", "-eu", "-c"]
    command:
      - |
        [ -z "$$TEO_KIN_DATABASE_URL" ] || { echo "external database (TEO_KIN_DATABASE_URL): restore it with your own tools, see docs/deployment.md"; exit 1; }
        src="$$RESTORE_FROM"
        if [ -z "$$src" ]; then
          for d in $$(ls -1 /backups | sort); do if [ -f "/backups/$$d/info.txt" ]; then src=$$d; fi; done
        fi
        dir="/backups/$$src"
        [ -n "$$src" ] && [ -f "$$dir/info.txt" ] && [ -f "$$dir/db.dump" ] && [ -f "$$dir/files.tar.gz" ] || { echo "no finished backup: $$dir (need info.txt, db.dump, files.tar.gz)"; exit 1; }
        pg_restore -l "$$dir/db.dump" > /dev/null
        tar -tzf "$$dir/files.tar.gz" > /dev/null
        busy=$$(psql -d postgres -tAc "SELECT count(*) FROM pg_stat_activity WHERE datname = '$$PGDATABASE' AND pid <> pg_backend_pid()")
        if [ "$$busy" != 0 ] && [ -z "$$RESTORE_FORCE" ]; then
          echo "the application is connected ($$busy sessions): stop app and retry (RESTORE_FORCE=1 skips the check)"; exit 1
        fi
        echo "database <- $$dir/db.dump"
        dropdb --if-exists --force "$$PGDATABASE"
        createdb "$$PGDATABASE"
        pg_restore -d "$$PGDATABASE" --no-owner --exit-on-error "$$dir/db.dump"
        echo "files <- $$dir/files.tar.gz"
        find /data -mindepth 1 -delete
        tar -xzf "$$dir/files.tar.gz" -C /data --numeric-owner
        echo "done: start app"

volumes:
  kintion-db:
  kintion-data:
  kintion-backups:
```

- **Variables.** In Dockge put them in the stack's `.env` / Environment box, in Portainer in "Environment variables". Set at least `TEO_KIN_DB_PASSWORD`; useful ones: `TEO_KIN_HTTP_BIND` (port, default `8080`), `TEO_KIN_PUBLIC_URL`, `TEO_KIN_LANGUAGE` (`en` or `ru`), `TEO_KIN_PACKS`. The full list is in [Configuration](#configuration).
- **First start.** Deploy with a build (the first build takes a few minutes). The setup token is in the `app` log: Dockge — the console/logs of the stack, Portainer — container `app` → Logs; or `docker compose logs app | grep bootstrap_token`. Open `http://<host>:<port>`, enter the token and choose the owner login and password.
- **Update.** The revision is pinned by the tag. To update, change the tag in `context` (`#v0.1.0` → the new one) **and** set `TEO_KIN_VERSION` to the same value: the image tag changes, so the redeploy builds the new image (a plain "re-pull" does nothing here, the image is built locally, not pulled from a registry). If the tag is unchanged, rebuild from the stack terminal: `docker compose build --no-cache app && docker compose up -d`. Read the release notes first: [Migrations and upgrades](#migrations-and-upgrades).
- **Backup.** `backup` and `restore` are in the `tools` profile and never start with the stack (do not enable the profile for the whole stack: that would start `restore` too). Run the backup from the stack terminal (Dockge: "Terminal"; elsewhere any shell in the stack directory): `docker compose run --rm backup` — naming the service enables its profile. Portainer has no per-stack terminal for this: on the Docker host run `docker compose -p <stack name> -f <the stack's compose file> run --rm backup` (Portainer keeps stack files in its data volume, under `compose/<stack id>/`; or save the file above on the host and use the same project name). Pass the same variables to that command (`TEO_KIN_DB_PASSWORD=... docker compose ...` or `--env-file`): values set only in the Portainer UI are not visible to the shell, and a wrong password makes the backup fail. The same goes for `restore`. Unlike the repository's compose file, backups go to the named volume `kintion-backups` (Compose prefixes its name with the stack name; see `docker volume ls`) instead of the `./backups` folder, which Portainer stacks do not have. To copy them out (take the real volume name, ending in `_kintion-backups`, from `docker volume ls`): `docker run --rm -v <volume>:/b -v "$PWD":/o alpine cp -r /b /o/backups`. Restore works as in [Backup and restore](#backup-and-restore), with `/backups` being that volume, not a host folder (stop `app`, then `docker compose run --rm restore`). The cron example there is for the repository setup.
- **Scope.** Docker Standalone only: `docker stack deploy` (Swarm) ignores `build`.

## Configuration

All variables start with `TEO_KIN_`. Priority: defaults → YAML file from `TEO_KIN_CONFIG` → environment. YAML keys are the lower-case names without the prefix (`http_bind`, `database_url`…).

| Variable | Default | Meaning |
| --- | --- | --- |
| `TEO_KIN_HTTP_BIND` | `:8080` | listen address or port |
| `TEO_KIN_PUBLIC_URL` | — | public address, `http(s)://host`; used for Secure cookies and Origin checks |
| `TEO_KIN_DATABASE_URL` | bundled `db` | PostgreSQL connection string |
| `TEO_KIN_DATABASE_URL_FILE` | — | file with the connection string (runs without the stock compose) |
| `TEO_KIN_STORAGE_PATH` | `/data` | files folder |
| `TEO_KIN_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `TEO_KIN_LANGUAGE` | `en` | instance language: `ru`, `en` |
| `TEO_KIN_PACKS` | `kin.core` | enabled bundled packs, comma-separated |
| `TEO_KIN_PACKS_PATH` | — | read packs from disk instead of the embedded ones (pack development) |
| `TEO_KIN_AUTH_MODE` | `local` | sign-in; only `local` is supported |
| `TEO_KIN_REGISTRATION` | `closed` | only `closed` (single owner) is supported |
| `TEO_KIN_UPLOAD_MAX_MB` | `20` | upload size limit |
| `TEO_KIN_CONFIG` | — | path to a YAML settings file |

Compose-only variables (not passed to the application): `TEO_KIN_DB_PASSWORD`, `TEO_KIN_VERSION`.

### External PostgreSQL

Set `TEO_KIN_DATABASE_URL=postgres://user:password@host:5432/kintion?sslmode=require`. The bundled `db` still starts but is unused. The `backup` and `restore` services refuse to run with an external database — use your own tools.

## Migrations and upgrades

The schema is upgraded automatically at start (migrations are embedded in the binary). If the database schema is newer than the application, the application refuses to start. Manually:

```sh
docker compose exec app /usr/local/bin/kintion migrate status
docker compose exec app /usr/local/bin/kintion migrate down    # roll back one migration
```

To upgrade: make a backup, get the new version, rebuild:

```sh
git pull
TEO_KIN_VERSION=<version> docker compose up -d --build
```

`TEO_KIN_VERSION` is recorded in the image and in every change-log row. To roll back, check out the previous version; if the new one already applied migrations, roll them back with the **new** image first (`migrate down`, one at a time), or restore the backup taken before the upgrade.

## Backup and restore

Data lives in two places:

| What | Where | Saved as |
| --- | --- | --- |
| Database: records, values, relations, groups, change log, account, sessions | PostgreSQL, volume `kintion-db` | `pg_dump -Fc` |
| Files: photos and previews | volume `kintion-data` → `/data` | `tar.gz` |

A backup is **both**: image values refer to files by hash. Packs are built into the image and are not part of a backup.

**Backup** (the application keeps running):

```sh
docker compose run --rm backup
```

This creates `backups/<date-time>-<n>/` with `db.dump`, `files.tar.gz` and `info.txt` (time, schema and application versions; written last, so a folder without it is unfinished). In Dockge or Portainer it is the `backup` service of the stack; copy the `backups/` folder off the server (rsync, scp, Syncthing). Schedule it with the host's cron, for example daily at 03:00:

```cron
0 3 * * * cd /opt/kintion && docker compose run --rm backup >>/var/log/kintion-backup.log 2>&1
```

**Restore** replaces the database and the files:

1. Stop **only** `app`; `db` must stay up. A running application makes `restore` refuse.
2. `docker compose run --rm restore` — the latest finished backup; a particular one: `RESTORE_FROM=<folder in backups/> docker compose run --rm restore`.
3. Start `app`. Restore a backup into the same or a newer application version, not an older one.

Keep backups off the server and test a restore now and then.

Without the services, by hand:

```sh
C="docker compose"
$C exec -T db pg_dump -U kintion -d kintion -Fc > db.dump
docker run --rm --volumes-from "$($C ps -a -q app)" --entrypoint tar postgres:18-alpine -czf - -C /data . > files.tar.gz
```

## Stop

```sh
docker compose down        # volumes stay
docker compose down -v     # also deletes the data — irreversible
```
