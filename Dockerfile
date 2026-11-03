# syntax=docker/dockerfile:1

# vango 框架 CLI 镜像。
#
#   docker build -t vango:dev .
#   docker run --rm vango:dev version
#
# 构建期可通过 build-arg 注入版本信息。

ARG GO_VERSION=1.22

FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_AT=unknown

# 先只拷依赖清单，依赖不变时复用缓存层。
COPY go.mod go.sum* ./
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
      -o /out/vango ./cmd/vango

FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_AT=unknown

# OCI 元数据，便于镜像仓库与编排系统展示来源。
LABEL org.opencontainers.image.title="vango" \
      org.opencontainers.image.description="vango Go 应用框架 CLI" \
      org.opencontainers.image.source="https://github.com/funxdata/vango" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.created="${BUILD_AT}"

COPY --from=build /out/vango /usr/local/bin/vango

# distroless 自带 nonroot 用户（uid/gid 65532）。
USER nonroot:nonroot

# 停止容器时由 vango 自己处理 SIGTERM 完成优雅关闭。
STOPSIGNAL SIGTERM

ENTRYPOINT ["/usr/local/bin/vango"]
CMD ["help"]
