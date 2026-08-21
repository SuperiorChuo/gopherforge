# Build auth-service with Go 1.27.
# fast-deploy 快路径：--build-arg MAIN_SRC=prebuilt 时产物直接取本机交叉编译的 .bin/main，
# BuildKit 只构建被引用的阶段，容器内编译（builder）整段跳过；缺省仍在容器内编译。
ARG MAIN_SRC=builder
FROM golang:1.27-alpine AS builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

# 构建上下文是 microservices/services：shared 拷到 /shared，命中 go.mod 的 replace ../shared（/app/../shared）。
# Single-module: go.mod at services/ root (module convergence 2026-08).
COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY shared/ /shared/
COPY auth/ .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o main ./cmd/main.go

# Run the auth-service on a pinned Alpine image as an unprivileged user.
# Configuration is environment-only; migrations are owned by the monolith.
FROM alpine:3.20 AS prebuilt
COPY auth/.bin/main /app/main

FROM ${MAIN_SRC} AS main-src

FROM alpine:3.20

WORKDIR /app

# tzdata is required: the libpq DSN pins TimeZone=Asia/Shanghai and pgx
# resolves it via the local zoneinfo database.
RUN apk add --no-cache tzdata \
    && addgroup -S app && adduser -S app -G app && mkdir -p ./logs && chown -R app:app /app

COPY --from=main-src --chown=app:app /app/main .

USER app

EXPOSE 8082

CMD ["./main"]
