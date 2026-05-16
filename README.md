# JProxy Go

A lightweight Go rewrite of JProxy for Sonarr/Radarr indexer query optimization.

This project is intended as a direct replacement for the original Java service
on NAS Docker deployments. It keeps the same default port, reuses the legacy UI,
and stores mutable data in the same SQLite `jproxy.db` style database.

## Features

- Sonarr/Radarr query proxy routes:
  - `/sonarr/jackett/*`
  - `/sonarr/prowlarr/*`
  - `/radarr/jackett/*`
  - `/radarr/prowlarr/*`
- Legacy web UI served from `web-dist`
- No-login compatibility for the legacy UI
- Config, cache, sync, rule, title, TMDB and example management APIs
- Sonarr/Radarr title sync
- Sonarr/Radarr rule sync
- TMDB title sync
- Built-in scheduler
- Pure-Go SQLite driver
- Small `scratch` Docker runtime image

## Docker Compose

For NAS deployment, mount `jproxy.db` as writable because the service updates
config, synced titles and rules.

```yaml
services:
  jproxy:
    image: your-dockerhub-name/jproxy-go:latest
    container_name: jproxy
    restart: always
    environment:
      - TZ=Asia/Shanghai
      - CORE_PROXY_ADDR=:8117
      - CORE_PROXY_MIN_COUNT=6
      - JPROXY_DB_PATH=/data/jproxy.db
      - WEB_DIST_PATH=/app/web-dist
      # Optional outbound proxy for GitHub/TMDB/etc.
      # - HTTP_PROXY=http://192.168.50.12:7897
      # - HTTPS_PROXY=http://192.168.50.12:7897
    ports:
      - 8117:8117
    volumes:
      - /vol1/1000/docker/media-manager/jproxy/database/jproxy.db:/data/jproxy.db
```

If your existing path is a directory such as:

```text
/vol1/1000/docker/media-manager/jproxy/database
```

then mount the actual database file inside it:

```yaml
volumes:
  - /vol1/1000/docker/media-manager/jproxy/database/jproxy.db:/data/jproxy.db
```

## Configuration

- `JPROXY_DB_PATH`: SQLite database path. Default: `/data/jproxy.db` in Docker.
- `CORE_PROXY_ADDR`: listen address. Default: `:8117`.
- `CORE_PROXY_MIN_COUNT`: Sonarr fallback threshold. Default: `6`.
- `WEB_DIST_PATH`: legacy UI assets path. Default: `/app/web-dist` in Docker.

## Verify Deployment

```bash
curl http://127.0.0.1:8117/healthz
docker logs --tail=100 jproxy
```

Expected health response:

```text
ok
```

## Local Development

```powershell
Set-Location D:\Project\jproxy-main\core-proxy
$env:JPROXY_DB_PATH="D:\Project\jproxy-main\src\main\resources\database\jproxy.db"
& "C:\Program Files\Go\bin\go.exe" run ./cmd/core-proxy
```

Run tests:

```powershell
$env:GOCACHE="$PWD\.gocache"
& "C:\Program Files\Go\bin\go.exe" test ./...
```

## Build Docker Locally

```bash
docker build -t jproxy-go:local .
docker run --rm -p 8117:8117 \
  -e JPROXY_DB_PATH=/data/jproxy.db \
  -v /path/to/jproxy.db:/data/jproxy.db \
  jproxy-go:local
```

## Automatic Docker Hub Publishing

This repository includes `.github/workflows/docker-publish.yml`.

To enable automatic builds:

1. Create a Docker Hub repository, for example `your-dockerhub-name/jproxy-go`.
2. In the GitHub repository, open `Settings -> Secrets and variables -> Actions`.
3. Add `DOCKERHUB_USERNAME`.
4. Add `DOCKERHUB_TOKEN`, preferably a Docker Hub access token rather than your password.
5. Push to `main` or create a tag such as `v0.1.0`.

The workflow publishes:

- `latest` for the default branch
- branch tags
- version tags such as `v0.1.0`
- commit tags such as `sha-xxxxxxx`

## NAS Update Flow

After automatic publishing is enabled, updating the NAS is just:

```bash
cd /vol1/1000/docker/media-manager
docker compose pull jproxy
docker compose up -d jproxy
```

No manual file copy is needed.

## Compatibility Status

The Go service currently covers the original controller API surface used by the
legacy UI, plus the main Sonarr/Radarr indexer proxy paths.

Implemented:

- indexer query rewrite and XML formatting
- rule import/export/save/remove/enable/disable/sync
- title query/remove/sync
- TMDB title query/save/remove/sync
- example query/save/remove
- system config query/update/version/author list
- cache clear endpoints
- no-login user endpoints

Known remaining migration area:

- qBittorrent/Transmission downloader login and automatic torrent/file rename
  background tasks. These are not part of the UI controller API, but they exist
  in the original Java service and should be migrated if you rely on them.
