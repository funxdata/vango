GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_AT ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X github.com/funxdata/vango/version.Version=$(VERSION) \
	-X github.com/funxdata/vango/version.GitCommit=$(GIT_COMMIT) \
	-X github.com/funxdata/vango/version.BuildAt=$(BUILD_AT)

IMAGE ?= vango
IMAGE_TAG ?= $(VERSION)
EXAMPLE_IMAGE ?= vango-hello
COMPOSE_FILE ?= examples/hello/docker-compose.yml

.PHONY: all build cli test test-race cover vet fmt fmt-check lint tidy check \
	docker docker-example compose-up compose-down compose-check clean help

all: check

## build: 编译全部包
build:
	$(GO) build ./...

## cli: 构建 vango 命令行到 bin/vango
cli:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/vango ./cmd/vango

## test: 运行单元测试
test:
	$(GO) test ./...

## test-race: 开启竞态检测运行测试
test-race:
	$(GO) test -race ./...

## cover: 生成覆盖率报告
cover:
	$(GO) test -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -func=coverage.out | tail -n 1

## vet: 静态检查
vet:
	$(GO) vet ./...

## fmt: 格式化代码
fmt:
	gofmt -w .

## fmt-check: 检查格式，不改写文件
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then echo "以下文件未格式化："; echo "$$unformatted"; exit 1; fi

## lint: 运行 golangci-lint
lint:
	golangci-lint run

## tidy: 整理依赖
tidy:
	$(GO) mod tidy

## check: 提交前的完整检查
check: fmt-check vet test

## docker: 构建 CLI 镜像
docker:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_AT=$(BUILD_AT) \
		-t $(IMAGE):$(IMAGE_TAG) .

## docker-example: 构建示例应用镜像
docker-example:
	docker build -f examples/hello/Dockerfile -t $(EXAMPLE_IMAGE):dev .

## compose-up: 用 compose 启动示例（后台）
compose-up:
	docker compose -f $(COMPOSE_FILE) up --build -d

## compose-down: 停止并清理 compose 资源
compose-down:
	docker compose -f $(COMPOSE_FILE) down

## compose-check: 校验 compose 文件
compose-check:
	docker compose -f $(COMPOSE_FILE) config --quiet

## clean: 清理构建与覆盖率产物
clean:
	rm -rf bin coverage.out

## help: 列出可用目标
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
