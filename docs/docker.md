# 在 Docker 中运行 vango

vango 的默认选择就是为容器设计的：单二进制、静态链接、零第三方依赖、
SIGTERM 优雅关闭、内置健康检查端点、配置走环境变量。本文说明如何把 vango 应用
打包成生产可用的镜像，以及容器环境下的几处注意事项。

## 1. 应用镜像

`vango new` 生成的项目自带 `Dockerfile`，核心是两阶段构建：

```dockerfile
# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags "-s -w -X github.com/funxdata/vango/version.Version=v1.0.0" \
      -o /out/app .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
ENV VANGO_HTTP_ADDR=:8080
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app", "healthcheck"]
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app"]
```

要点：

- **`CGO_ENABLED=0`**：静态二进制，可以直接放进 `scratch` / distroless。
- **`-trimpath -ldflags "-s -w"`**：去掉构建路径与调试信息，镜像更小。
- **distroless `nonroot`**：无 shell、无包管理器、非 root 运行，攻击面最小。
- **BuildKit 缓存**：`--mount=type=cache` 让 `go mod` 与编译缓存跨构建复用；
  文件头的 `# syntax=docker/dockerfile:1` 是启用该语法的前提。
- **`COPY go.mod` 先行**：依赖未变时复用缓存层。生成 go.sum 后可以改成
  `COPY go.mod go.sum ./`，让依赖校验参与缓存键。
- **`-X .../version.Version`**：在镜像构建期注入版本信息，`/healthz` 会返回它。
- **`STOPSIGNAL SIGTERM`**：容器停止时把 SIGTERM 交给应用自己处理。

构建与运行：

```bash
docker build -t my-api:dev .
docker run --rm -p 8080:8080 -e VANGO_LOG_FORMAT=json my-api:dev
```

## 2. 配置：环境变量优先

容器里不该烘焙配置文件，vango 的配置支持环境变量覆盖：

| 配置项 | 环境变量 | 默认 |
| --- | --- | --- |
| `http.addr` | `VANGO_HTTP_ADDR` | `:8080` |
| `log.level` | `VANGO_LOG_LEVEL` | `info` |
| `log.format` | `VANGO_LOG_FORMAT` | `json` |
| `db.dsn` | `VANGO_DB_DSN` | — |

```bash
docker run --rm -p 8080:8080 \
  -e VANGO_HTTP_ADDR=:8080 \
  -e VANGO_DB_DSN="postgres://vango:vango@db:5432/vango?sslmode=disable" \
  my-api:dev
```

`vango.New` 在未显式设置地址时按 `WithAddress` > `http.addr`（含 `VANGO_HTTP_ADDR`）
> `PORT` > `:8080` 的顺序决定监听地址，兼容需要动态端口的托管平台。

## 3. 健康探针

distroless 镜像里没有 `curl`，因此 vango 提供一个子命令做探针：

```dockerfile
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["/app", "healthcheck"]
```

```go
if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
    if err := healthcheck.Run(""); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
    return
}
```

探针地址按以下优先级推导，改了端口不需要改 Dockerfile：

| 环境变量 | 作用 | 默认 |
| --- | --- | --- |
| `VANGO_HEALTHCHECK_URL` | 完整地址，优先级最高 | — |
| `VANGO_HEALTHCHECK_PATH` | 探针路径，可改为 `/readyz` | `/healthz` |
| `VANGO_HEALTHCHECK_TIMEOUT` | 单次请求超时（`time.ParseDuration` 格式） | `3s` |
| `VANGO_HTTP_ADDR` | 应用监听地址，探针从中取端口 | — |
| `PORT` | 托管平台注入的端口 | `8080` |

探针始终直连 `127.0.0.1`，忽略 `HTTP_PROXY` / `HTTPS_PROXY`，避免容器内代理配置
导致误报不健康。

Kubernetes 中可以直接复用同样的端点：

```yaml
livenessProbe:
  httpGet: { path: /healthz, port: 8080 }
readinessProbe:
  httpGet: { path: /readyz, port: 8080 }
```

`/readyz` 会 `Ping` 数据库，适合判断「可以接流量」。

### 探针日志

容器平台每隔几秒就请求一次探针，默认访问日志会被 `/healthz` 刷屏。
用 `WithLogSkipPaths` 跳过它们，其余日志不受影响：

```go
app := vango.New(
    vango.WithLogSkipPaths("/healthz", "/readyz"),
)
```

## 4. 真实客户端 IP 与限流

容器前面通常还有一层反向代理（nginx、Traefik、云负载均衡），`RemoteAddr` 会是
代理地址。默认的 `Context.IP()` 会读取 `X-Forwarded-For`，但该头可以被客户端伪造。

启用 `RealIP` 后，只有直连对端位于可信网段时才采信转发头，并把结果写回 Context，
之后的 `IP()`、访问日志、`RateLimit` 都使用真实客户端地址：

```go
router.Use(
    web.RealIP(web.TrustedProxies()...), // 127/8、10/8、172.16/12、192.168/16、::1
    web.RateLimit(50, 100),
)
```

`web.TrustedProxies()` 覆盖 Docker bridge 与 Kubernetes Pod 的常见网段；暴露在公网时
建议用 `web.ParseTrustedProxies("10.0.0.0/8")` 按实际拓扑收窄。

## 5. 优雅关闭

容器编排会把 `SIGTERM` 发给 PID 1，vango 的 `Run()` 收到信号后：

1. 停止接收新连接；
2. 等待进行中的请求完成（默认最多 15s，`vango.WithShutdownTimeout` 可调）；
3. 逆序执行 `OnStop` 钩子；
4. 关闭数据库连接。

`docker stop` 的默认宽限期是 10s，小于 vango 的 15s 关闭超时。因此：

- Dockerfile 里声明 `STOPSIGNAL SIGTERM`；
- compose 里设置 `stop_grace_period: 20s`（或 `docker stop -t 20`）；
- 用 `init: true` 让 tini 作为 PID 1，正确转发信号并回收僵尸进程。

若应用自己就是 PID 1 且没有 tini，Go 运行时会忽略部分信号的默认行为，
`init: true` 仍是最省心的选择。

## 6. 数据库与启动顺序

示例见 `examples/hello/docker-compose.yml`：

```bash
docker compose -f examples/hello/docker-compose.yml up --build
curl http://localhost:8080/api/v1/greet/vango
```

需要数据库时，取消 compose 文件里 `db` 服务的注释，并给 api 服务加上：

```yaml
environment:
  VANGO_DB_DSN: "postgres://vango:vango@db:5432/vango?sslmode=disable"
depends_on:
  db:
    condition: service_healthy
```

数据库在启动时可能还没就绪，因此 `db.Open` 的 `Ping` 失败应当被视为可重试错误
（或使用 `depends_on` 的 `service_healthy` 条件）。推荐的启动钩子写法：

```go
app.OnStart("db", func(ctx context.Context) error {
    return retry(ctx, 10*time.Second, func() error { return database.PingContext(ctx) })
})
```

## 7. 日志

容器里日志应写到 stdout/stderr，由运行时收集。vango 默认输出 JSON 到 stdout：

```json
{"time":"2026-11-03T10:00:00Z","level":"INFO","msg":"request","method":"GET","path":"/api/v1/greet/vango","status":200,"duration_ms":0.42,"ip":"10.0.0.7","request_id":"..."}
```

失败请求的 `status` 是最终写入响应的状态码（例如 404/500），而不是 200。
不要在生产镜像里把日志写文件；需要文件采集时由 sidecar 处理。

## 8. 镜像瘦身清单

- 使用 `golang:1.22-alpine` 构建，最终镜像用 distroless `static`。
- 不要 `COPY .` 到最终镜像，只拷二进制。
- `.dockerignore` 排除 `.git`、文档与测试产物。
- 构建阶段用 `--mount=type=cache` 缓存 `go mod` 与编译产物。
- 需要时区数据时用 `--tags timetzdata` 或 distroless 的 `static`（已含 tzdata）。
- 需要 HTTPS 出网时，distroless `static` 已内置 CA 证书。

## 9. CI 中构建镜像

`.github/workflows/ci.yml` 中的 `docker` 任务会构建 CLI 镜像与示例镜像，
确保 Dockerfile 不会随代码演进失效。本地等价命令：

```bash
make docker          # 构建 CLI 镜像
make docker-example  # 构建示例应用镜像
make compose-check   # 校验 compose 文件
```
