// Command hello 是 vango 的最小示例：只有 JSON API 与数据层。
package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/funxdata/vango"
	"github.com/funxdata/vango/config"
	"github.com/funxdata/vango/healthcheck"
	"github.com/funxdata/vango/web"
)

func main() {
	// 容器探针：/app healthcheck
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := healthcheck.Run(""); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	cfg := config.New(config.WithValues(map[string]any{
		"http": map[string]any{"addr": ":8080"},
		"log":  map[string]any{"level": "info", "format": "text"},
	}))

	app := vango.New(
		vango.WithName("hello"),
		vango.WithConfig(cfg),
		vango.WithServerOptions(
			web.WithTimeouts(15*time.Second, 30*time.Second, 60*time.Second),
		),
		vango.WithMiddleware(web.CORS(), web.SecureHeaders()),
		// 容器探针每几秒请求一次，不必写入访问日志。
		vango.WithLogSkipPaths("/healthz", "/readyz"),
	)

	app.GET("/", func(c *web.Context) error {
		return web.OK(c, map[string]string{"hello": "vango"})
	})

	api := app.Group("/api/v1")

	// 路径参数
	api.GET("/greet/:name", func(c *web.Context) error {
		return web.OK(c, map[string]string{"greeting": "hello, " + c.Param("name")})
	})

	// JSON 请求体绑定与校验
	api.POST("/echo", func(c *web.Context) error {
		var payload struct {
			Message string `json:"message"`
		}
		if err := c.BindAndValidate(&payload); err != nil {
			return err
		}
		return web.Created(c, payload)
	})

	if err := app.Run(); err != nil {
		slog.Error("app exited", "error", err)
		os.Exit(1)
	}
}
