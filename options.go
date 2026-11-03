package vango

import (
	"log/slog"
	"time"

	"github.com/funxdata/vango/config"
	"github.com/funxdata/vango/db"
	"github.com/funxdata/vango/web"
)

// Option 定制 App 的构建。
type Option func(*App)

// WithName 设置应用名。
func WithName(name string) Option {
	return func(a *App) { a.name = name }
}

// WithVersion 覆盖应用版本，默认取 version.Get()。
func WithVersion(v string) Option {
	return func(a *App) { a.version = v }
}

// WithAddress 设置监听地址，优先级高于配置项 http.addr。
func WithAddress(addr string) Option {
	return func(a *App) {
		a.addr = addr
		a.addrSet = true
	}
}

// WithConfig 注入配置。配置中的 http.addr、log.level、log.format 会被应用读取。
func WithConfig(cfg *config.Config) Option {
	return func(a *App) { a.config = cfg }
}

// WithLogger 注入日志器，默认按配置创建 JSON 日志。
func WithLogger(logger *slog.Logger) Option {
	return func(a *App) { a.logger = logger }
}

// WithDB 注入数据库连接，应用关闭时会自动 Close。
func WithDB(database *db.DB) Option {
	return func(a *App) { a.database = database }
}

// WithShutdownTimeout 设置优雅关闭的最长等待时间，默认 15s。
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(a *App) { a.shutdownTimeout = timeout }
}

// WithServerOptions 透传 HTTP 服务选项，例如超时与 TLS。
func WithServerOptions(opts ...web.ServerOption) Option {
	return func(a *App) { a.serverOptions = append(a.serverOptions, opts...) }
}

// WithMiddleware 追加全局中间件，在默认中间件之后执行。
func WithMiddleware(mw ...web.Middleware) Option {
	return func(a *App) { a.middlewares = append(a.middlewares, mw...) }
}

// WithLogSkipPaths 指定不记录访问日志的路径，例如容器探针 /healthz、/readyz。
//
// 与 WithoutDefaultMiddleware 不同，它只减少日志噪声，其余内置中间件照常生效。
func WithLogSkipPaths(paths ...string) Option {
	return func(a *App) { a.logSkipPaths = append(a.logSkipPaths, paths...) }
}

// WithoutDefaultMiddleware 关闭内置的 Recover/RequestID/Logger 中间件。
func WithoutDefaultMiddleware() Option {
	return func(a *App) { a.defaultMiddleware = false }
}

// WithoutHealthEndpoints 关闭内置的 /healthz 与 /readyz。
func WithoutHealthEndpoints() Option {
	return func(a *App) { a.healthEndpoints = false }
}
