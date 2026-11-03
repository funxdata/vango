package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

// Options 描述脚手架参数。
type Options struct {
	// Name 是项目名。
	Name string
	// ModulePath 是项目 module 路径。
	ModulePath string
	// VangoVersion 是 go.mod 中依赖的 vango 版本。
	VangoVersion string
}

var projectNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// Scaffold 在 dir 下生成一个新项目。
//
// 目录已存在且非空时返回错误，避免覆盖用户代码。
func Scaffold(dir string, opts Options) error {
	if !projectNamePattern.MatchString(opts.Name) {
		return fmt.Errorf("invalid project name %q", opts.Name)
	}
	if opts.ModulePath == "" {
		opts.ModulePath = opts.Name
	}
	if opts.VangoVersion == "" {
		opts.VangoVersion = "v0.1.0"
	}

	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("directory %s is not empty", dir)
	}

	files := map[string]string{
		"go.mod":             goModTemplate,
		"main.go":            mainTemplate,
		"config.json":        configTemplate,
		"Dockerfile":         dockerfileTemplate,
		".dockerignore":      dockerignoreTemplate,
		"docker-compose.yml": composeTemplate,
		"README.md":          readmeTemplate,
		".gitignore":         gitignoreTemplate,
	}

	for name, content := range files {
		rendered, err := render(name, content, opts)
		if err != nil {
			return err
		}

		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func render(name, content string, opts Options) (string, error) {
	tmpl, err := template.New(name).Parse(content)
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", name, err)
	}

	var builder strings.Builder
	if err := tmpl.Execute(&builder, opts); err != nil {
		return "", fmt.Errorf("render template %s: %w", name, err)
	}
	return builder.String(), nil
}

const goModTemplate = `module {{.ModulePath}}

go 1.22

require github.com/funxdata/vango {{.VangoVersion}}
`

const mainTemplate = `package main

import (
	"fmt"
	"log/slog"
	"os"

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

	app := vango.New(
		vango.WithName("{{.Name}}"),
		vango.WithConfig(config.MustLoad("config.json")),
		// 容器探针每几秒请求一次，不必写入访问日志。
		vango.WithLogSkipPaths("/healthz", "/readyz"),
	)

	app.GET("/api/v1/ping", func(c *web.Context) error {
		return web.OK(c, map[string]string{"pong": "{{.Name}}"})
	})

	if err := app.Run(); err != nil {
		slog.Error("app exited", "error", err)
		os.Exit(1)
	}
}
`

const configTemplate = `{
  "http": {
    "addr": ":8080"
  },
  "log": {
    "level": "info",
    "format": "json"
  }
}
`

const dockerfileTemplate = `# syntax=docker/dockerfile:1

ARG GO_VERSION=1.22

FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_AT=unknown

# 只有 go.mod 时依赖层缓存依旧有效；执行过 go mod tidy 后
# 可以改成 COPY go.mod go.sum ./ 以获得可复现的依赖校验。
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags "-s -w \
        -X github.com/funxdata/vango/version.Version=${VERSION} \
        -X github.com/funxdata/vango/version.GitCommit=${GIT_COMMIT} \
        -X github.com/funxdata/vango/version.BuildAt=${BUILD_AT}" \
      -o /out/app .

FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_AT=unknown

LABEL org.opencontainers.image.title="{{.Name}}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.created="${BUILD_AT}"

COPY --from=build /out/app /app

ENV VANGO_HTTP_ADDR=:8080
EXPOSE 8080

USER nonroot:nonroot

# 探针从 VANGO_HTTP_ADDR 推导端口，容器内不需要 curl。
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app", "healthcheck"]

STOPSIGNAL SIGTERM

ENTRYPOINT ["/app"]
`

const dockerignoreTemplate = `bin/
dist/
coverage.out
*.test

.git/
.github/
.idea/
.vscode/

config.local.json
*.pem
*.key
*.ssh
`

const composeTemplate = `services:
  app:
    build:
      context: .
    image: {{.Name}}:dev
    ports:
      - "8080:8080"
    # tini 作为 PID 1，确保信号与僵尸进程被正确处理。
    init: true
    # 应用默认 15s 优雅关闭，比 docker stop 的 10s 默认宽限期更长。
    stop_grace_period: 20s
    environment:
      VANGO_HTTP_ADDR: ":8080"
      VANGO_LOG_FORMAT: "json"
    healthcheck:
      test: ["CMD", "/app", "healthcheck"]
      interval: 10s
      timeout: 3s
      retries: 3
      start_period: 5s
    restart: unless-stopped
`

const readmeTemplate = `# {{.Name}}

由 vango 脚手架生成：一个只提供 JSON API 的服务。

## 本地运行

` + "```bash" + `
go mod tidy
go run .
` + "```" + `

## Docker

` + "```bash" + `
docker build -t {{.Name}}:dev .
docker run --rm -p 8080:8080 {{.Name}}:dev

# 或
docker compose up --build
` + "```" + `

## 接口

` + "```bash" + `
curl http://localhost:8080/api/v1/ping
curl http://localhost:8080/healthz
` + "```" + `

## 配置

配置来自 ` + "`config.json`" + `，并可用环境变量覆盖（前缀 ` + "`VANGO_`" + `），
例如 ` + "`VANGO_HTTP_ADDR=:9000`" + `。
`

const gitignoreTemplate = `# 构建产物
/bin/
/dist/

# 本地配置
config.local.json

# 编辑器
.idea/
.vscode/
*.swp
`
