# JProxy Go

JProxy 的轻量级 Go 版本,由AI重写(**Powered by Codex**)

**因为内存实在是太贵了，我实在买不起，就只好重写项目了**

## 致谢
本项目基于 [LuckyPuppy514/jproxy](https://github.com/LuckyPuppy514/jproxy) 用 Go 语言重写，感谢原作者的贡献。功能真的很好。

## 功能特性
- 参考原项目就好，应该都一比一地实现了

## Docker Compose

NAS 部署时，需要将 `jproxy.db` 以可写方式挂载，因为服务会更新配置、同步标题和规则。

```yaml
services:
  jproxy:
    image: 4thhydrogen/jproxy-go:latest
    container_name: jproxy
    restart: always
    environment:
      - TZ=Asia/Shanghai
      - CORE_PROXY_ADDR=:8117
      - CORE_PROXY_MIN_COUNT=6
      - JPROXY_DB_PATH=/data/jproxy.db
      - WEB_DIST_PATH=/app/web-dist
      # 可选：出站代理，记得改成你的代理地址
      # - HTTP_PROXY=http://192.168.50.12:7897
      # - HTTPS_PROXY=http://192.168.50.12:7897
    ports:
      - 8117:8117
    volumes:
      - /vol1/1000/docker/media-manager/jproxy/database/jproxy.db:/data/jproxy.db
```


## 配置项

- `JPROXY_DB_PATH`：SQLite 数据库路径。Docker 环境默认：`/data/jproxy.db`。
- `CORE_PROXY_ADDR`：监听地址。默认：`:8117`。
- `CORE_PROXY_MIN_COUNT`：Sonarr 回退阈值。默认：`6`。
- `WEB_DIST_PATH`：UI 静态资源路径。Docker 环境默认：`/app/web-dist`。

## 验证部署

```bash
curl http://127.0.0.1:8117/healthz
docker logs --tail=100 jproxy
```

健康检查预期响应：

```text
ok
```

## 本地开发

```powershell
Set-Location D:\Project\jproxy-main\core-proxy
$env:JPROXY_DB_PATH="D:\Project\jproxy-main\src\main\resources\database\jproxy.db"
& "C:\Program Files\Go\bin\go.exe" run ./cmd/core-proxy
```

运行测试：

```powershell
$env:GOCACHE="$PWD\.gocache"
& "C:\Program Files\Go\bin\go.exe" test ./...
```

## 本地构建 Docker

```bash
docker build -t jproxy-go:local .
docker run --rm -p 8117:8117 \
  -e JPROXY_DB_PATH=/data/jproxy.db \
  -v /path/to/jproxy.db:/data/jproxy.db \
  jproxy-go:local
```

## Docker Hub 自动发布

本仓库已配置 `.github/workflows/docker-publish.yml` 自动构建工作流。

启用自动构建的步骤：

1. 在 Docker Hub 创建仓库，例如 `your-dockerhub-name/jproxy-go`。
2. 在 GitHub 仓库中，进入 `Settings -> Secrets and variables -> Actions`。
3. 添加 `DOCKERHUB_USERNAME`。
4. 添加 `DOCKERHUB_TOKEN`，推荐使用 Docker Hub Access Token 而非密码。
5. 推送到 `main` 分支或创建 tag（如 `v0.1.0`）即可触发构建。

工作流会发布以下标签：

- `latest` — 默认分支最新版本
- 分支标签
- 版本标签，如 `v0.1.0`
- 提交 SHA 标签，如 `sha-xxxxxxx`


## 兼容性状态

Go 服务目前已覆盖原有 UI 控制器使用的全部 API，以及主要的 Sonarr/Radarr indexer 代理路径。

已实现：

- indexer 查询重写和 XML 格式化
- 规则导入/导出/保存/删除/启用/禁用/同步
- 标题查询/删除/同步
- TMDB 标题查询/保存/删除/同步
- 示例查询/保存/删除
- 系统配置查询/更新/版本/作者列表
- 缓存清理接口
- 免登录用户接口（`isLoginEnabled` 返回 `false`，自动跳过登录）

待迁移：

- qBittorrent/Transmission 下载器登录及自动种子/文件重命名后台任务。
  这些功能不属于 UI 控制器 API，但存在于原 Java 服务中，如有需要应进行迁移。

