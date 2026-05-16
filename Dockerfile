FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/core-proxy ./cmd/core-proxy

FROM scratch
WORKDIR /app
ENV CORE_PROXY_ADDR=:8117
ENV CORE_PROXY_MIN_COUNT=6
ENV JPROXY_DB_PATH=/data/jproxy.db
ENV WEB_DIST_PATH=/app/web-dist
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/core-proxy /app/core-proxy
COPY web-dist /app/web-dist
EXPOSE 8117
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/app/core-proxy", "healthcheck"]
ENTRYPOINT ["/app/core-proxy"]
