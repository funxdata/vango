# syntax=docker/dockerfile:1

# 示例应用的镜像：构建上下文是仓库根目录。
#
#   docker build -f examples/hello/Dockerfile -t vango-hello .
#   docker run --rm -p 8080:8080 vango-hello

ARG GO_VERSION=1.22

FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_AT=unknown

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
      -o /out/app ./examples/hello

FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_AT=unknown

LABEL org.opencontainers.image.title="vango-hello" \
      org.opencontainers.image.description="vango 示例应用" \
      org.opencontainers.image.source="https://github.com/funxdata/vango" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.created="${BUILD_AT}"

COPY --from=build /out/app /app

ENV VANGO_HTTP_ADDR=:8080
EXPOSE 8080

USER nonroot:nonroot

# 探针直接从 VANGO_HTTP_ADDR 推导端口，无需 curl。
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app", "healthcheck"]

STOPSIGNAL SIGTERM

ENTRYPOINT ["/app"]
