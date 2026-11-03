# 架构说明

vango 的目标是「把 JSON API 的样板代码收拢成一套约定」，同时保持标准库的透明度。
本文说明分层、依赖方向与扩展点。

## 1. 分层

```
                   ┌──────────────────────────┐
   进程入口 main   │  vango.New(...) + App     │  生命周期 / 模块 / 钩子
                   └────────────┬─────────────┘
                                │ 装配
        ┌───────────────────────┼────────────────────────┐
        │                       │                        │
   ┌────▼─────┐          ┌──────▼──────┐          ┌──────▼─────┐
   │  web     │          │    auth     │          │    db      │
   │ 路由/上下文│          │ JWT/密码/鉴权│          │ SQL/事务/ORM│
   └────┬─────┘          └──────┬──────┘          └──────┬─────┘
        │                       │                        │
        └───────────┬───────────┴────────────┬───────────┘
                    │                        │
              ┌─────▼─────┐            ┌─────▼─────┐
              │   errs    │            │  config   │
              │ 结构化错误 │            │  配置     │
              └───────────┘            └───────────┘
                    │                        │
              ┌─────▼─────┐            ┌─────▼─────┐
              │    log    │            │  version  │
              └───────────┘            └───────────┘
```

依赖方向严格单向：上层依赖下层，**下层不知道上层存在**。
`config` 不依赖 `log`，`web` 不依赖 `db`，`errs` 不依赖任何框架包。

## 2. 一次请求的生命周期

```
TCP ──▶ web.Server ──▶ web.Router.ServeHTTP
                          │
                          ├─ 全局中间件（Recover → RequestID → Logger → CORS ...）
                          │
                          ├─ 路由匹配（段树：静态 / :param / *wildcard）
                          │
                          ├─ 分组中间件（auth.Middleware、RequireRoles ...）
                          │
                          ├─ HandlerFunc(c *web.Context) error
                          │      ├─ c.BindAndValidate(&payload)   JSON 解析 + 校验
                          │      ├─ db.QueryAll[T](ctx, database, sql, args...)
                          │      └─ web.OK(c, data) / web.Created(c, data)
                          │
                          └─ 返回 error ──▶ errs.From(err) ──▶ web.Response（JSON）
```

要点：

- handler 只返回 `error`，不直接写响应头。错误统一由 `errs.From` 映射成
  `{"code": <http status>, "message": ..., "error_code": <业务码>, "request_id": ...}`。
- `error_code` 来自 `errs.Error.Code`，是稳定的机器可读标识；`code` 只反映 HTTP 状态码。
- 未包装的 error 一律变成 500，原始错误保留在 `errors.Is/As` 链与日志中，
  不会泄漏给客户端。
- 响应只可能是 JSON 或 204，框架不提供 HTML/静态文件能力。

## 3. 数据层

`db` 是 `database/sql` 的薄封装，不隐藏标准库：

| 能力 | API |
| --- | --- |
| 连接与连接池 | `db.Open(driver, dsn, db.WithMaxOpenConns(...))` |
| 事务 | `database.Tx(ctx, func(tx *sql.Tx) error)` |
| 结构体 → 行 | `db.Insert` / `db.Update` / `db.Delete` / `db.Count` |
| 行 → 结构体 | `db.ScanAll[T](rows)` / `db.ScanOne[T](rows)` |
| 查询 + 映射 | `db.QueryAll[T](ctx, q, sql, args...)` / `db.QueryOne[T]` |
| 方言 | `db.WithDialect(db.Dollar)`（PostgreSQL 用 `$n`） |

字段映射顺序：`db` tag → `json` tag → 字段名的 snake_case。
`db:"id,auto"` 表示自增列（插入时跳过），`db:"id,pk"` 表示主键（更新时跳过），
`db:"-"` 表示忽略。

SQL 标识符会做白名单校验；`Update`/`Delete` 拒绝空 `where`，避免误伤全表。

## 4. 扩展点

| 扩展点 | 用法 |
| --- | --- |
| `Option` | `vango.New(vango.WithAddress(...), vango.WithDB(...))` |
| `Module` | `app.Register(mymodule)`，模块内注册路由与钩子 |
| `Middleware` | `app.Use(...)` / `app.Group(prefix, ...)` |
| `Validator` | 业务结构体实现 `Validate() error`，`BindAndValidate` 自动调用 |
| `ServerOption` | `vango.WithServerOptions(web.WithTimeouts(...), web.WithTLS(...))` |
| 生命周期钩子 | `app.OnStart(name, fn)` / `app.OnStop(name, fn)` |

模块示例：

```go
type userModule struct{ database *db.DB }

func (m userModule) Name() string { return "user" }

func (m userModule) Register(app *vango.App) error {
	api := app.Group("/api/v1/users")
	api.GET("/:id", m.get)
	api.POST("", m.create)
	return nil
}
```

## 5. 启动与关闭顺序

```
Start:
  1. net.Listen（端口占用立即报错）
  2. 依次执行 OnStart 钩子（任一失败则中止，已启动的钩子由调用方负责清理）
  3. 后台 Serve

Shutdown（收到 SIGTERM / context 取消 / Serve 出错）:
  1. http.Server.Shutdown（停止接收新连接，等待在途请求，上限 shutdownTimeout）
  2. 逆序执行 OnStop 钩子
  3. db.Close
```

`Run()` 把这些串起来并阻塞，因此容器里只要把 `SIGTERM` 发给进程即可优雅退出。

## 6. 部署形态

vango 应用就是一个静态二进制：

- 编译期注入版本：`-X github.com/funxdata/vango/version.Version=...`
- 运行期配置：环境变量优先（`VANGO_*`）
- 健康检查：`/healthz`（存活）、`/readyz`（含数据库 Ping）
- 容器探针：`/app healthcheck` 子命令（见 `vango/healthcheck`）
- 日志：JSON 写 stdout

详见 [docker.md](docker.md)。

## 7. 为什么零依赖

- 供应链更小：`go.mod` 没有 `require`，不存在第三方漏洞传导。
- 构建更快、更可预测：`go build` 只需要标准库。
- 升级成本低：约束来自 Go 语言本身。

代价是部分能力需要自己实现（JWT、PBKDF2、路由、配置）。这些实现都保持小而可读，
并且有对应的单元测试；确实超出范围的（如 SSH 客户端）不再内置，交由业务方按需引入。
