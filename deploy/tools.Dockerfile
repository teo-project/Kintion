# Тулбокс для машины без Go: go, golangci-lint, sqlc, goose.
# Версия Go должна совпадать с go.mod и deploy/Dockerfile.
FROM golang:1.27

ARG GOLANGCI_LINT_VERSION=v2.14.0
ARG SQLC_VERSION=v1.31.1
ARG GOOSE_VERSION=v3.28.0

RUN go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION} \
 && go install github.com/sqlc-dev/sqlc/cmd/sqlc@${SQLC_VERSION} \
 && go install github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION} \
 && rm -rf /root/.cache/go-build /go/pkg/mod

WORKDIR /src
