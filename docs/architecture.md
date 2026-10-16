# Architecture

## Overview

```text
 browser (Vue 3 SPA) ──HTTP/JSON──▶ kintion (Go) ──SQL──▶ PostgreSQL
                                       │
                                       └── local volume /data (photos and previews)
```

- One Go binary serves the JSON API under `/api/`, the files, `/healthz`, and the built single-page application (embedded in the binary).
- PostgreSQL is the only database. Nothing else is required: no queue, cache or search engine.
- Files (photos and their crops) live on a local volume. They are served by the Go process, which checks access on every request — there are no public file URLs.
- The **domain knowledge** (what a person is, which fields a marriage has, how roles are named) is not in the code. It is loaded from **packs** (see [packs.md](packs.md)); the code only implements mechanisms.

## Stack

| Layer | Choice |
| --- | --- |
| Backend | Go, standard `net/http.ServeMux` (method-aware patterns), no web framework |
| Database access | [sqlc](https://sqlc.dev) — typed queries generated from SQL |
| Migrations | goose, embedded in the binary, applied at start |
| Database | PostgreSQL 18 |
| Frontend | Vue 3, TypeScript, Vite |
| Canvas | Vue Flow for the tree canvas; the tree layout algorithm is custom |
| UI kit | Reka UI, Lucide icons, light and dark theme (the "Nocturne" style) |
| Delivery | Docker Compose: app + PostgreSQL + volumes |

## Repository layout

| Path | What |
| --- | --- |
| `server/cmd/` | the `kintion` binary: server, `migrate`, `reset-password`, `healthcheck` |
| `server/internal/api` | HTTP handlers |
| `server/internal/records` | records, fields, participation, groups, tree, trash |
| `server/internal/packs` | pack loader and validation |
| `server/internal/partialdate` | the partial date type (Go side) |
| `server/internal/audit` | change log |
| `server/internal/auth`, `authz` | sessions, passwords, access checks |
| `server/internal/db` | migrations, sqlc queries |
| `server/internal/files` | file storage with preview generation |
| `web/` | the Vue application |
| `pack-storage/<lang>/<pack>/` | bundled packs |
| `i18n/` | interface strings of the application (`en.json`, `ru.json`) |
| `deploy/` | Dockerfile, Compose files |
| `e2e/` | end-to-end tests (Playwright) |

## API

JSON over HTTP, cookie session with a CSRF token. Main resources (all under `/api`):

| Method and path | Purpose |
| --- | --- |
| `GET /setup`, `POST /setup` | first start: create the owner with the one-time setup token |
| `POST /login`, `POST /logout`, `GET /me`, `POST /me/password` | session |
| `GET/PUT /me/prefs` | per-account preferences (theme, card mode, tree view) |
| `GET/POST /dashboards`, `PATCH /dashboards/{d}` | dashboards |
| `GET /dashboards/{d}/schema` | templates and fields of the dashboard, resolved from packs |
| `PUT /dashboards/{d}/templates/{t}/custom_fields` | shared custom fields of a template |
| `PUT /dashboards/{d}/templates/{t}/quick_fields` | quick-add form fields of a template |
| `GET /dashboards/{d}/records` | list with filters, search and template-defined sort |
| `GET /dashboards/{d}/tree` | data for the tree view |
| `POST /dashboards/{d}/operations` | create a record with initial values, participants, groups in one operation |
| `GET/PATCH/DELETE /records/{id}` | a record |
| `POST /records/{id}/restore`, `DELETE /records/{id}/purge` | trash |
| `POST/PATCH/DELETE /records/{id}/fields/{key}/values[/{vid}]` | values of a field |
| `POST/PATCH/DELETE /records/{id}/participants[/{pid}]` | participation in a relationship or event |
| `GET/POST/PATCH/DELETE /dashboards/{d}/groups…`, `PUT /records/{id}/groups` | groups |
| `GET /dashboards/{d}/audit` | change log |
| `POST /dashboards/{d}/files`, `GET /files/{d}/{sha}[/preview]` | files, addressed by content hash |
| `GET /healthz` | `200` with `"db":"ok"` when the app and the database are up, otherwise `503` |

Concurrent editing is handled per field: every field has a revision counter, and an edit made against a stale revision is rejected so the user can decide instead of silently overwriting.

## Principles the code follows

- **Nothing answers differently depending on invisible data.** No uniqueness constraints on domain data; identifiers are UUIDs; counts and search only see what the viewer may see. This keeps the model ready for sharing.
- **Missing is not invented.** No placeholder records for unknown people; an unknown part of a date stays unknown.
- **Packs carry data, code carries mechanisms.** No pack ids or field keys are hard-coded for domain behaviour.
- **Schema changes are forward migrations with data transfer.** Existing migrations are never edited.
- **Everything is logged.** Every change goes into the audit log with the operation, the old and the new value, the actor and the application version.

## Tests

Go unit and integration tests (against a real PostgreSQL), web unit tests, and a Playwright end-to-end scenario that sets up a fresh installation and walks the main flows. A separate script checks the whole backup → wipe → restore cycle.
