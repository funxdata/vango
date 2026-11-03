# vango

**vango** 是一个用 Go 编写的 **API 与数据层框架**：零第三方依赖、只做 JSON 接口与数据库访问、
天生面向容器部署。

它不包含 HTML 模板、静态文件、前端资源或 Cookie 会话 —— 服务端只产出 JSON，
前端交给独立的前端工程。这让框架很小，也让它很适合跑在 Docker / Kubernetes 里。

```go
package main

import (
	"log/slog"
	"os"

	"github.com/funxdata/vango"
	"github.com/funxdata/vango/web"
)

func main() {
	app := vango.New(vango.WithName("demo"))

	app.GET("/api/v1/hello/:name", func(c *web.Context) error {
		return web.OK(c, map[string]string{"hello": c.Param("name")})
	})

	if err := app.Run(); err != nil {
		slog.Error("app exited", "error", err)
		os.Exit(1)
	}
}
```

```bash
curl http://localhost:8080/api/v1/hello/vango
# {"code":0,"message":"ok","data":{"hello":"vango"},"request_id":"..."}
```

---

## 特性

- **只做 JSON API**：请求体是 JSON，查询参数用 `BindQuery`，响应统一为 JSON 信封。
- **数据层优先**：`database/sql` 薄封装 + 轻量 ORM（结构体映射、CRUD、`ScanAll[T]`、`QueryAll[T]`）。
- **零第三方依赖**：`go.mod` 没有任何 `require`，只需要 Go 1.22+。
- **容器原生**：静态单二进制、非 root 镜像、SIGTERM 优雅关闭、`/healthz` `/readyz`、
  内置 `healthcheck` 探针子命令、环境变量优先的配置。
- **应用生命周期**：`vango.App` 负责路由装配、模块注册、启动/关闭钩子与优雅关闭。
- **路由与中间件**：路径段树路由（`/users/:id`、`/files/*path`）、路由分组，
  内置 Recover / RequestID / Logger / CORS / Timeout / BodyLimit / RateLimit。
- **统一错误**：`errs.Error` 携带 HTTP 状态码与业务错误码；未知错误统一 500 且不泄露内部细节。
- **鉴权**：JWT（HS256/384/512）、PBKDF2-SHA256 密码哈希、Bearer 中间件与角色校验。
- **日志**：标准库 `log/slog`，默认输出 JSON 到 stdout，便于容器日志采集。

## 安装

```bash
go get github.com/funxdata/vango
```

## 快速开始

```bash
go install github.com/funxdata/vango/cmd/vango@latest

vango new demo
cd demo
go mod tidy
go run .
```

`vango new` 会生成 `main.go`、`config.json`、`Dockerfile`、`docker-compose.yml`、
`.dockerignore` 与 README，生成即可直接 `docker compose up --build`。

## Docker

整个应用就是容器里的一个静态二进制，详细说明见 [docs/docker.md](docs/docker.md)。

```bash
# 构建并运行示例
make docker-example
docker run --rm -p 8080:8080 vango-hello:dev

# 或者直接使用 compose
make compose-up
curl http://localhost:8080/api/v1/greet/vango
```

应用镜像采用两阶段构建：`golang:1.22-alpine` 编译 → `gcr.io/distroless/static-debian12:nonroot` 运行。

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/app .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
ENV VANGO_HTTP_ADDR=:8080
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["/app", "healthcheck"]
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app"]
```

容器相关的约定：

| 关注点 | vango 的做法 |
| --- | --- |
| 监听地址 | `WithAddress` > `http.addr` > `PORT` > `:8080`，默认绑 `0.0.0.0` |
| 配置 | 环境变量优先：`VANGO_HTTP_ADDR`、`VANGO_LOG_FORMAT`、`VANGO_DB_DSN` … |
| 健康检查 | `/healthz`（存活）、`/readyz`（含数据库 Ping）；distroless 镜像用 `/app healthcheck` |
| 探针地址 | `VANGO_HEALTHCHECK_URL` > `VANGO_HTTP_ADDR` > `PORT` > `:8080`；路径/超时可用 `VANGO_HEALTHCHECK_PATH`、`VANGO_HEALTHCHECK_TIMEOUT` 调整 |
| 真实客户端 IP | `web.RealIP(web.TrustedProxies()...)`，仅在可信代理后采信转发头，供日志与限流使用 |
| 探针日志 | `vango.WithLogSkipPaths("/healthz", "/readyz")`，避免探针刷屏访问日志 |
| 优雅关闭 | 收到 `SIGTERM` 后停止接收连接、等待在途请求、逆序执行 `OnStop`、关闭数据库 |
| 运行参数 | Dockerfile 声明 `STOPSIGNAL SIGTERM`，compose 使用 `init: true` 与 `stop_grace_period: 20s` |
| 日志 | JSON 写 stdout，由运行时采集 |

## 核心概念

### App 与生命周期

```go
app := vango.New(
	vango.WithName("orders"),
	vango.WithDB(database),
	vango.WithShutdownTimeout(20*time.Second),
)

app.OnStart("cache", func(ctx context.Context) error { return cache.Warm(ctx) })
app.OnStop("cache", func(ctx context.Context) error { cache.Close(); return nil })

app.Register(user.Module, order.Module)
app.Run()
```

启动顺序：绑定端口（失败同步返回）→ 依次执行启动钩子 → 提供服务。
关闭顺序：停止接收新请求 → 逆序执行关闭钩子 → 关闭数据库。

### 路由

```go
api := app.Group("/api/v1", auth.Middleware(signer))

api.GET("/users/:id", getUser)
api.GET("/orders", listOrders)
api.POST("/users", createUser)
api.DELETE("/users/:id", deleteUser)
```

### 请求绑定

请求体只解析 JSON，查询参数单独绑定：

```go
type CreateOrder struct {
	UserID int64   `json:"user_id"`
	Amount float64 `json:"amount"`
}

func createOrder(c *web.Context) error {
	var payload CreateOrder
	if err := c.BindAndValidate(&payload); err != nil {
		return err // 自动转换为 400 + 统一错误体
	}
	return web.Created(c, payload)
}

func listOrders(c *web.Context) error {
	var query struct {
		Page   int    `query:"page"`
		Size   int    `query:"size"`
		Status string `query:"status"`
	}
	if err := c.BindQuery(&query); err != nil {
		return err
	}
	// ...
	return web.OKPage(c, items, total, query.Page, query.Size)
}
```

### 数据层

```go
type User struct {
	ID    int64  `db:"id,auto"`
	Name  string `db:"name"`
	Email string `db:"email"`
}

// 写入
_, err := db.Insert(ctx, database, "users", user)

// 查询并映射
users, err := db.QueryAll[User](ctx, database,
	"SELECT id, name, email FROM users WHERE tenant_id = $1 LIMIT $2", tenantID, limit)

user, err := db.QueryOne[User](ctx, database,
	"SELECT id, name, email FROM users WHERE id = $1", id)

// 事务
err = database.Tx(ctx, func(tx *sql.Tx) error {
	_, err := db.Update(ctx, tx, "users", user, "id = ?", user.ID)
	return err
})
```

复杂 SQL 直接用 `*sql.DB`，vango 不做隐藏。

### 错误与响应

```go
return errs.NotFound("user not found")                  // 404
return errs.Conflict("email already exists")            // 409
return errs.Internal("db unavailable").WithCause(err)   // 500，原因只进日志
```

成功与失败统一为：`{"code":0,"message":"ok","data":...}` /
`{"code":404,"message":"...","error_code":"NOT_FOUND","request_id":"..."}`。

`code` 与 HTTP 状态码一致；`error_code` 是稳定的业务错误码（来自 `errs.Error.Code`），
客户端应优先基于它分支，避免依赖描述文案。

### 鉴权

```go
signer, _ := auth.NewSigner([]byte(secret), auth.WithIssuer("vango")) // secret 至少 32 字节
token, _ := signer.Sign(auth.Claims{Subject: "42", Roles: []string{"admin"}}, time.Hour)

api := app.Group("/api", auth.Middleware(signer, auth.WithSkipPaths("/api/login")))
admin := api.Group("/admin", auth.RequireRoles("admin"))
```

## 包一览

| 包 | 说明 |
| --- | --- |
| `vango` | 应用装配、生命周期、模块系统 |
| `vango/web` | 路由、Context、中间件、JSON 响应、HTTP Server |
| `vango/db` | `database/sql` 封装、事务、轻量 ORM、泛型查询 |
| `vango/auth` | JWT、密码哈希、鉴权中间件 |
| `vango/config` | JSON + 环境变量配置 |
| `vango/log` | 基于 `slog` 的日志器 |
| `vango/errs` | 结构化错误与 HTTP 映射 |
| `vango/healthcheck` | 容器健康探针 |
| `vango/httpx` | 出站 HTTP 客户端（JSON/表单/文件/代理） |
| `vango/version`、`vango/debug` | 构建版本信息、SIGUSR1 运行时诊断 |
| `vango/internal/cli` | `vango new` 脚手架（内部包，不对外） |

## 设计原则

1. **只做数据与 API**：不引入模板、静态资源、前端构建，服务端输出只有 JSON。
2. **标准库优先**：零第三方依赖；每引入一个依赖都要有充分理由。
3. **显式优于隐式**：没有全局单例与运行时魔法，应用由 `vango.App` 显式装配。
4. **容器优先**：静态二进制、非 root、环境变量配置、健康探针、优雅关闭都是默认行为。
5. **可拆分**：每个包都能独立使用，`web` 不依赖 `db`，`config` 不依赖 `log`。

## 路线图

- [ ] `vango/cache`：内存与 Redis 抽象
- [ ] `vango/queue`：任务队列
- [ ] OpenAPI 生成与请求/响应校验
- [ ] 数据库迁移工具

## 贡献

欢迎 issue 与 PR，请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 与
[行为准则](CODE_OF_CONDUCT.md)。安全问题请按 [SECURITY.md](SECURITY.md) 私下报告。

## 许可证

[MIT](LICENSE)
