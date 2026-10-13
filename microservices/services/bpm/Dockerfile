# fast-deploy 快路径：--build-arg MAIN_SRC=prebuilt 时产物直接取本机交叉编译的 .bin/main，
# BuildKit 只构建被引用的阶段，容器内编译（builder）整段跳过；缺省仍在容器内编译。
ARG MAIN_SRC=builder
FROM golang:1.27-alpine AS builder
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn
# 依赖下载层只拷两侧 go.mod/go.sum，改源码不打穿模块缓存；模块与编译缓存挂 BuildKit cache mount 做增量。
# Single-module: go.mod at services/ root (module convergence 2026-08).
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o main ./bpm/cmd/main.go

FROM alpine:3.20 AS prebuilt
COPY bpm/.bin/main /app/main

FROM ${MAIN_SRC} AS main-src

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache tzdata wget \
    && addgroup -S app && adduser -S app -G app \
    && chown -R app:app /app
COPY --from=main-src --chown=app:app /app/main .
USER app
EXPOSE 8096
CMD ["./main"]
