# Build system-service with Go 1.27.
# fast-deploy 快路径：--build-arg MAIN_SRC=prebuilt 时产物直接取本机交叉编译的 .bin/main，
# BuildKit 只构建被引用的阶段，容器内编译（builder）整段跳过；缺省仍在容器内编译。
ARG MAIN_SRC=builder
FROM golang:1.27-alpine AS builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

# 构建上下文是 microservices：shared 拷到 /shared，命中 go.mod 的 replace ../shared（/app/../shared）。
# 依赖下载层只拷两侧 go.mod/go.sum，改源码不打穿模块缓存；模块与编译缓存挂 BuildKit cache mount 做增量。
# Single-module: go.mod at services/ root
COPY services/go.mod services/go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY services/ .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o main ./system/cmd/main.go

# 预检与 ZIP 只需要这些接入源文件；快照在构建时固化，不挂载宿主仓库。
# 独立成阶段（不放 builder 里）：prebuilt 快路径也要快照，但不该为它触发容器内编译。
FROM alpine:3.20 AS snapshot
COPY services/system/internal/api/routes.go /repository-snapshot/microservices/services/system/internal/api/routes.go
COPY web/src/router/index.tsx /repository-snapshot/microservices/web/src/router/index.tsx
COPY web/src/layouts/menu-defs.tsx /repository-snapshot/microservices/web/src/layouts/menu-defs.tsx
COPY services/system/internal/service/system/menu_seed.go /repository-snapshot/microservices/services/system/internal/service/system/menu_seed.go
COPY services/monitor/migrations/ /repository-snapshot/microservices/services/monitor/migrations/

# Run the system-service on a pinned Alpine image as an unprivileged user.
# Configuration is environment-only; migrations are owned by the monolith.
FROM alpine:3.20 AS prebuilt
COPY services/system/.bin/main /app/main

FROM ${MAIN_SRC} AS main-src

FROM alpine:3.20

WORKDIR /app

# tzdata is required: the libpq DSN pins TimeZone=Asia/Shanghai and pgx
# resolves it via the local zoneinfo database.
RUN apk add --no-cache tzdata \
	&& addgroup -S -g 10001 app \
	&& adduser -S -D -H -u 10001 -G app app \
	&& mkdir -p ./logs \
	&& chown -R app:app /app

COPY --from=main-src --chown=app:app /app/main .
COPY --from=snapshot /repository-snapshot /app/repository-snapshot

ENV CODEGEN_REPO_ROOT=/app/repository-snapshot \
    CODEGEN_WRITE_ENABLED=false

USER app

EXPOSE 8084

CMD ["./main"]
