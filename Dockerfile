# syntax=docker/dockerfile:1
FROM golang:1.27.3-bookworm AS build
WORKDIR /src
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
RUN go generate ./internal/views
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/main-service .

FROM docker.io/library/caddy:2-alpine AS gateway
COPY --from=build /out/main-service /usr/bin/main-service
ENTRYPOINT ["/usr/bin/main-service", "gateway"]
CMD ["-config", "/etc/caddy/Caddyfile", "-output", "/srv/main"]

FROM debian:bookworm-slim AS main
RUN apt-get update && apt-get install --yes --no-install-recommends ca-certificates curl \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --gid 10001 app && useradd --uid 10001 --gid app --no-create-home app \
 && mkdir -p /app /srv/main && chown -R app:app /app /srv/main
WORKDIR /app
COPY --from=build /out/main-service ./
USER 10001:10001
EXPOSE 8082
ENTRYPOINT ["/app/main-service"]
