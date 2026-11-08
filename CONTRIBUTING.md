# Contributing to Kintion

Thank you for wanting to help. Kintion is a small project with one maintainer, so please read this page first: it saves time on both sides.

## Before you start

- **Bugs:** open an issue with the "Bug report" form. Steps to reproduce, the version (Account → About) and what you expected matter most.
- **Ideas and larger changes:** open an issue with the "Feature request" form first and wait for an answer before writing code. Kintion follows a set of design decisions (for example, nothing may behave differently depending on data the user cannot see; domain knowledge lives in packs, not in code), and a change that conflicts with them cannot be merged however good the code is.
- **Security problems:** do not open a public issue, see [SECURITY.md](SECURITY.md).
- **Small fixes** (typos, an obvious bug with a test) can go straight to a pull request.

## Contributor License Agreement

Every contributor signs the [Contributor License Agreement](CLA.md) once. You keep the copyright in your work; the agreement lets the project distribute it, including under other licenses in the future.

How to sign: open a pull request; the CLA bot posts a comment with a link to the agreement. If you agree, reply in the pull request with exactly:

```text
I have read the CLA Document and I hereby sign the CLA
```

The check turns green and stays green for all your later pull requests. A pull request without a signature is not merged. If you contribute on behalf of your employer, make sure you are allowed to (section 4 of the agreement).

## Development setup

Needs Docker with Compose v2 and Node.js 24. Go is optional: the toolbox container has Go, golangci-lint, sqlc and goose.

```sh
# Go: vet, lint, tests (a test PostgreSQL starts automatically)
docker compose -f deploy/compose.dev.yml run --rm tools sh -c "go vet ./... && golangci-lint run && go test ./..."

# Web
cd web && npm ci && npm run typecheck && npm run lint && npm test && npm run build

# End-to-end test on a fresh Compose install (port 18080)
e2e/run.sh
```

After changing SQL queries or migrations, regenerate the code: `docker compose -f deploy/compose.dev.yml run --rm tools sqlc generate -f server/internal/db/sqlc.yaml`. The layout of the code is described in [docs/architecture.md](docs/architecture.md).

## Pull requests

- One topic per pull request; keep unrelated refactoring out.
- Add or update tests for what you change. CI must be green: Go (vet, lint, tests, generated sqlc code), web (types, lint, tests, build) and the end-to-end test.
- **Database schema** changes only through a **new** migration that also moves existing data; existing migrations are never edited.
- **Language:** issues, pull requests and commit messages in English or Russian. Code, identifiers and keys are in English. Existing comments in the code are in Russian; new comments may be in either language.
- User interface texts go through `i18n/` in both languages (`en`, `ru`); if you cannot write one of them, say so in the pull request.
- Do not commit secrets or real personal data; examples use made-up people.

## Review

The maintainer reviews every pull request; expect questions and change requests. Merging needs a green CI, a signed CLA and an approving review. A pull request may be closed if it conflicts with the project's design decisions; the reason is always explained.
