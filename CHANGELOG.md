# 更新日志

本项目遵循[语义化版本](https://semver.org/lang/zh-CN/)，格式参考
[Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)。

## [Unreleased]

### Added

- `vango/healthcheck`：容器健康探针，地址按 `VANGO_HEALTHCHECK_URL` → `VANGO_HTTP_ADDR`
  → `PORT` → `:8080` 推导，路径与超时可用 `VANGO_HEALTHCHECK_PATH` /
  `VANGO_HEALTHCHECK_TIMEOUT` 调整，探针直连本机、不受代理环境变量影响。
- `vango/web`：`RealIP` 中间件与 `TrustedProxies` / `ParseTrustedProxies`，
  只在直连对端可信时采信 `X-Forwarded-For` / `X-Real-IP`，还原真实客户端 IP。
- `vango/web`：`SecureHeaders` 中间件、`LoggerWithOptions`（可跳过探针路径的访问日志）。
- `vango`：`WithLogSkipPaths`，跳过 `/healthz`、`/readyz` 等探针的访问日志。
- `vango/web`：错误响应新增 `error_code`（来自 `errs.Error.Code`）稳定的业务错误码。
- 容器支持：Dockerfile 使用 BuildKit 缓存挂载、OCI 元数据标签与 `STOPSIGNAL`；
  compose 增加 `init` 与 `stop_grace_period`；`web.Server` 的 listener 访问加锁。
- `vango/db`：泛型查询 `QueryAll[T]` / `QueryOne[T]` 与 `Exec` 辅助函数。
- Docker 支持：CLI 与示例应用的多阶段 `Dockerfile`（distroless + nonroot）、
  `.dockerignore`、`docker-compose.yml`、`docs/docker.md`，以及 CI 镜像构建与冒烟测试。
- `vango new` 脚手架生成的工程默认包含 `Dockerfile`、`docker-compose.yml`、
  `.dockerignore` 与 `healthcheck` 探针子命令。
- 监听地址新增 `PORT` 环境变量回退（优先级：`WithAddress` > `http.addr` > `PORT` > `:8080`）。
- `vango`：`App` 应用装配、生命周期钩子、模块系统、优雅启停、内置 `/healthz` 与 `/readyz`。
- `vango/web`：路径段树路由（`:param`、`*wildcard`）、路由分组、`Context`
  （JSON/表单/查询绑定、统一响应）、内置中间件（Recover、RequestID、Logger、CORS、
  Timeout、BodyLimit、RateLimit、SecureHeaders）、支持优雅关闭的 `Server`。
- `vango/config`：JSON 配置文件 + 环境变量覆盖 + 点号路径 + 子视图 + 类型化取值 + 结构体解码。
- `vango/log`：基于标准库 `log/slog` 的日志器。
- `vango/errs`：结构化错误与统一 HTTP 映射。
- `vango/auth`：JWT（HS256/HS384/HS512）、PBKDF2-SHA256 密码哈希、Bearer 鉴权中间件。
- `vango/db`：`database/sql` 封装、事务辅助、轻量 ORM（结构体映射与基础 CRUD）。
- `vango/httpx`：HTTP 客户端（JSON、表单、multipart、代理）。
- `cmd/vango`：`new` / `version` / `help` 命令与项目脚手架。

### Changed

- 项目定位收敛为 API 与数据层框架：只保留运行时所需的包，通用工具包不再随框架发布。
- 日志使用标准库 `log/slog`，不再引入第三方日志库。
- 版本信息改为构建期通过 `-ldflags -X` 注入。
- 配置统一由 `config.Load` / `config.MustLoad` 返回实例，不使用全局配置。

### Fixed

- `config`：按点号路径读取嵌套配置一直返回默认值（`lookup` 未逐层遍历）。
- `config`：`Sub` 视图未继承已加载的配置文件路径。
- `internal/cli`：`vango new -dir` 传入 `*string` 导致整个仓库无法编译。
- `db`：`UpdateMap` 忽略 `WithDialect`，PostgreSQL 下生成错误的占位符。
- `web`：访问日志中失败请求的 `status` 被记为 200。
- `vango`：启动钩子失败后端口未释放，应用无法再次 `Start`。
- `web`：`Server.Addr` 与 `Server.Start` 并发读写 listener。
- `debug`：Windows 桩文件命名错误，导致该平台下包内没有任何 API。
- 代码格式：多个文件未通过 `gofmt`（CI 的 `gofmt -l` 会失败）。

### Removed

- 历史工具包：`character`、`crypto`、`diff`、`file`、`maths`、`pagination`、`rand`、
  `times`、`validate` —— 与框架职责无关，需要时可由业务项目自行实现或使用第三方库。
- 兼容层：`claims`（`auth.Claims` 别名）、`http`（`httpx` 废弃 shim）、`crypto/ssh` 空占位包。
- 第三方依赖：`spf13/viper`、`sirupsen/logrus`、`dgrijalva/jwt-go`、`Masterminds/semver`、
  `golang.org/x/crypto`、`golang.org/x/net`、`gopkg.in/check.v1`。
- `go.sum`：模块不再有任何第三方依赖，无需校验文件。
