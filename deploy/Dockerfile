# syntax=docker/dockerfile:1
# Образ Kintion: сборка Vue → сборка Go со встроенным фронтендом → минимальный образ.
# Контекст сборки — корень репозитория (см. compose.yml).

FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
COPY i18n/ /src/i18n/
RUN npm run build

FROM golang:1.27-alpine AS server
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY i18n/ i18n/
COPY pack-storage/ pack-storage/
COPY server/ server/
COPY web/embed.go web/
COPY web/licenses/ web/licenses/
COPY --from=web /src/web/dist/ web/dist/
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false \
      -ldflags "-s -w -X kintion/server/internal/version.Version=${VERSION}" \
      -o /out/kintion ./server/cmd/kintion \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=server /out/kintion /usr/local/bin/kintion
COPY --from=server --chown=65532:65532 /out/data /data
ENV TEO_KIN_HTTP_BIND=:8080 \
    TEO_KIN_STORAGE_PATH=/data
EXPOSE 8080
USER 65532:65532
HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=3 \
  CMD ["/usr/local/bin/kintion", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/kintion"]
CMD ["serve"]
