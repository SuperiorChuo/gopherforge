# Build backend with Go 1.26.
# fast-deploy 快路径：--build-arg MAIN_SRC=prebuilt 时产物直接取本机交叉编译的 .bin/main，
# BuildKit 只构建被引用的阶段，容器内编译（builder）整段跳过；缺省仍在容器内编译。
ARG MAIN_SRC=builder
FROM golang:1.26-alpine AS builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

# 构建上下文是 microservices/services：shared 拷到 /shared，命中 go.mod 的
# replace ../shared（/app/../shared）。monitor 接 shared 后必须与其余 17 个
# 服务同构，否则 docker build 里 ../shared 不存在，go mod download 直接失败。
COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o main ./cmd/main.go \
    && go build -trimpath -o migrate ./cmd/migrate

# Run the backend on a pinned Alpine image as an unprivileged user.
FROM alpine:3.20 AS prebuilt
COPY monitor/.bin/main /app/main
COPY monitor/.bin/migrate /app/migrate

FROM ${MAIN_SRC} AS main-src

FROM alpine:3.20

WORKDIR /app

# tzdata is required: the libpq DSN pins TimeZone=Asia/Shanghai and pgx
# resolves it via the local zoneinfo database.
RUN apk add --no-cache tzdata \
    && addgroup -S app && adduser -S app -G app && mkdir -p ./logs && chown -R app:app /app

COPY --from=main-src --chown=app:app /app/main .
COPY --from=main-src --chown=app:app /app/migrate .
# 上下文换到 services/ 后，这三份资源要带 monitor/ 前缀
COPY --chown=app:app monitor/configs ./configs
COPY --chown=app:app monitor/migrations ./migrations
COPY --chown=app:app monitor/resources ./resources
# 仓库/rsync 常排除 configs/config.yaml；compose 靠环境变量覆盖密钥
RUN if [ ! -f ./configs/config.yaml ]; then cp ./configs/config.example.yaml ./configs/config.yaml; fi \
    && chown app:app ./configs/config.yaml

USER app

EXPOSE 8081

# 迁移由 compose 的一次性 migrate 服务执行（同一镜像跑 ./migrate up）
CMD ["./main"]
