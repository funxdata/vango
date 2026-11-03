// Package vango 是一个基于标准库的、零第三方依赖的 Go 应用框架。
//
// 提供应用生命周期、路由与中间件、配置、日志、JWT 鉴权、数据库访问与 CLI 脚手架，
// 同时保留标准库的直白：所有能力都可以单独使用，也可以整体装配成一个应用。
//
// 最小示例：
//
//	app := vango.New(vango.WithAddress(":8080"))
//	app.GET("/hello", func(c *web.Context) error {
//		return web.OK(c, "world")
//	})
//	if err := app.Run(); err != nil {
//		log.Fatal(err)
//	}
package vango

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/funxdata/vango/config"
	"github.com/funxdata/vango/db"
	"github.com/funxdata/vango/errs"
	"github.com/funxdata/vango/log"
	"github.com/funxdata/vango/version"
	"github.com/funxdata/vango/web"
)

// Hook 是生命周期钩子。
type Hook func(ctx context.Context) error

type namedHook struct {
	name string
	fn   Hook
}

// App 是 vango 应用。
//
// 它持有一个路由表、一个 HTTP 服务、可选的数据库连接与配置，
// 并通过生命周期钩子管理启动与关闭顺序。
type App struct {
	name            string
	version         string
	addr            string
	addrSet         bool
	shutdownTimeout time.Duration

	config   *config.Config
	logger   *slog.Logger
	router   *web.Router
	database *db.DB

	serverOptions     []web.ServerOption
	middlewares       []web.Middleware
	logSkipPaths      []string
	maxBodyBytes      int64
	defaultMiddleware bool
	healthEndpoints   bool

	startHooks []namedHook
	stopHooks  []namedHook

	mu      sync.Mutex
	server  *web.Server
	errCh   chan error
	started bool
}

// New 创建应用。
//
// 默认行为：监听 :8080、JSON 日志、内置 Recover/RequestID/Logger 中间件、
// 暴露 /healthz 健康检查。都可以通过 Option 覆盖。
func New(opts ...Option) *App {
	app := &App{
		name:              "vango",
		version:           version.Get(),
		addr:              ":8080",
		shutdownTimeout:   15 * time.Second,
		router:            web.NewRouter(),
		maxBodyBytes:      4 << 20,
		defaultMiddleware: true,
		healthEndpoints:   true,
		errCh:             make(chan error, 1),
	}

	for _, opt := range opts {
		opt(app)
	}

	if app.config == nil {
		app.config = config.New()
	}
	if app.logger == nil {
		app.logger = log.New(log.Options{
			Level:  app.config.String("log.level", "info"),
			Format: app.config.String("log.format", "json"),
		})
	}
	if !app.addrSet {
		// 优先级：WithAddress > 配置项 http.addr > 环境变量 PORT > 默认 :8080。
		switch {
		case app.config.Has("http.addr"):
			app.addr = app.config.String("http.addr", app.addr)
		case os.Getenv("PORT") != "":
			app.addr = ":" + os.Getenv("PORT")
		}
	}
	if app.database != nil {
		app.logger.Info("database connected", "driver", app.database.Driver())
	}

	if app.defaultMiddleware {
		app.middlewares = append([]web.Middleware{
			web.Recover(app.logger),
			web.SecureHeaders(),
			web.RequestID(),
			web.LoggerWithOptions(web.LoggerOptions{
				Logger:    app.logger,
				SkipPaths: app.logSkipPaths,
			}),
		}, app.middlewares...)
	}
	if app.maxBodyBytes > 0 {
		app.middlewares = append([]web.Middleware{web.BodyLimit(app.maxBodyBytes)}, app.middlewares...)
	}
	app.router.Use(app.middlewares...)
	if app.healthEndpoints {
		app.registerHealthEndpoints()
	}

	return app
}

// Name 返回应用名。
func (a *App) Name() string { return a.name }

// Version 返回应用版本。
func (a *App) Version() string { return a.version }

// Addr 返回配置的监听地址；服务启动后返回实际监听地址。
func (a *App) Addr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.server != nil {
		return a.server.Addr()
	}
	return a.addr
}

// Config 返回应用配置。
func (a *App) Config() *config.Config { return a.config }

// Log 返回应用日志器。
func (a *App) Log() *slog.Logger { return a.logger }

// DB 返回应用数据库连接，可能为 nil。
func (a *App) DB() *db.DB { return a.database }

// Router 返回底层路由，便于使用未在 App 上代理的能力。
func (a *App) Router() *web.Router { return a.router }

// Use 注册全局中间件。
func (a *App) Use(mw ...web.Middleware) *App {
	a.router.Use(mw...)
	return a
}

// Group 创建路由分组。
func (a *App) Group(prefix string, mw ...web.Middleware) *web.Group {
	return a.router.Group(prefix, mw...)
}

// GET 注册 GET 路由。
func (a *App) GET(pattern string, h web.HandlerFunc) *App {
	a.router.GET(pattern, h)
	return a
}

// POST 注册 POST 路由。
func (a *App) POST(pattern string, h web.HandlerFunc) *App {
	a.router.POST(pattern, h)
	return a
}

// PUT 注册 PUT 路由。
func (a *App) PUT(pattern string, h web.HandlerFunc) *App {
	a.router.PUT(pattern, h)
	return a
}

// PATCH 注册 PATCH 路由。
func (a *App) PATCH(pattern string, h web.HandlerFunc) *App {
	a.router.PATCH(pattern, h)
	return a
}

// DELETE 注册 DELETE 路由。
func (a *App) DELETE(pattern string, h web.HandlerFunc) *App {
	a.router.DELETE(pattern, h)
	return a
}

// OnStart 注册启动钩子，按注册顺序执行；任一钩子失败则终止启动。
func (a *App) OnStart(name string, fn Hook) *App {
	a.startHooks = append(a.startHooks, namedHook{name: name, fn: fn})
	return a
}

// OnStop 注册关闭钩子，按注册的逆序执行。
func (a *App) OnStop(name string, fn Hook) *App {
	a.stopHooks = append(a.stopHooks, namedHook{name: name, fn: fn})
	return a
}

// Register 注册并初始化模块，应在 Start 之前调用。
func (a *App) Register(modules ...Module) error {
	for _, module := range modules {
		if module == nil {
			continue
		}
		if err := module.Register(a); err != nil {
			return fmt.Errorf("register module %s: %w", module.Name(), err)
		}
		a.logger.Debug("module registered", "module", module.Name())
	}
	return nil
}

// Start 依次执行启动钩子并开始监听。
//
// Start 返回时端口已成功绑定，因此端口占用等错误会同步返回。
func (a *App) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.started {
		a.mu.Unlock()
		return errors.New("vango: app already started")
	}

	listener, err := net.Listen("tcp", a.addr)
	if err != nil {
		a.mu.Unlock()
		return fmt.Errorf("vango: listen %s: %w", a.addr, err)
	}

	serverOptions := append([]web.ServerOption{web.WithServerLogger(a.logger)}, a.serverOptions...)
	a.server = web.NewServer(a.addr, a.router, append(serverOptions, web.WithListener(listener))...)
	a.started = true
	a.mu.Unlock()

	if ctx == nil {
		ctx = context.Background()
	}

	for _, hook := range a.startHooks {
		if err := hook.fn(ctx); err != nil {
			// 启动失败时释放已绑定的端口并复位状态，允许修复后重新 Start。
			_ = listener.Close()
			a.mu.Lock()
			a.started = false
			a.server = nil
			a.mu.Unlock()
			return fmt.Errorf("vango: start hook %q: %w", hook.name, err)
		}
		a.logger.Debug("start hook done", "hook", hook.name)
	}

	go func() {
		a.errCh <- a.server.Start()
	}()

	a.logger.Info("vango app started",
		"name", a.name,
		"version", a.version,
		"addr", a.server.Addr(),
	)
	return nil
}

// Shutdown 优雅关闭 HTTP 服务并执行关闭钩子，钩子按注册的逆序运行。
func (a *App) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	server := a.server
	started := a.started
	a.started = false
	a.mu.Unlock()

	if !started || server == nil {
		return nil
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), a.shutdownTimeout)
		defer cancel()
	}

	var failures []error
	if err := server.Shutdown(ctx); err != nil {
		failures = append(failures, fmt.Errorf("shutdown http server: %w", err))
	}

	for i := len(a.stopHooks) - 1; i >= 0; i-- {
		hook := a.stopHooks[i]
		if err := hook.fn(ctx); err != nil {
			failures = append(failures, fmt.Errorf("stop hook %q: %w", hook.name, err))
		}
	}

	if a.database != nil {
		if err := a.database.Close(); err != nil {
			failures = append(failures, fmt.Errorf("close database: %w", err))
		}
	}

	a.logger.Info("vango app stopped", "name", a.name)
	return errors.Join(failures...)
}

// Stop 使用内置的关闭超时调用 Shutdown。
func (a *App) Stop() error { return a.Shutdown(nil) }

// Run 启动应用并阻塞，直到收到 SIGINT/SIGTERM、context 取消或服务出错。
func (a *App) Run() error { return a.RunContext(context.Background()) }

// RunContext 是带 context 的 Run。
func (a *App) RunContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := a.Start(ctx); err != nil {
		return err
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	var runErr error
	select {
	case <-ctx.Done():
		a.logger.Info("context canceled, shutting down")
	case sig := <-signals:
		a.logger.Info("signal received, shutting down", "signal", sig.String())
	case runErr = <-a.errCh:
		if runErr != nil {
			a.logger.Error("http server stopped unexpectedly", "error", runErr.Error())
		}
	}

	stopErr := a.Stop()
	if runErr != nil {
		return runErr
	}
	return stopErr
}

// Err 返回 HTTP 服务的退出结果。
func (a *App) Err() <-chan error { return a.errCh }

// Handler 返回路由的 http.Handler，便于直接用 httptest 测试。
func (a *App) Handler() http.Handler { return a.router }

func (a *App) registerHealthEndpoints() {
	a.router.GET("/healthz", func(c *web.Context) error {
		return web.OK(c, map[string]string{
			"status":  "ok",
			"name":    a.name,
			"version": a.version,
		})
	})
	a.router.GET("/readyz", func(c *web.Context) error {
		if a.database != nil {
			ctx, cancel := context.WithTimeout(c.StdContext(), 2*time.Second)
			defer cancel()
			if err := a.database.PingContext(ctx); err != nil {
				return errs.Unavailable("database not ready").WithCause(err)
			}
		}
		return web.OK(c, map[string]string{"status": "ready"})
	})
}
