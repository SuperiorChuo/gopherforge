# 统一服务镜像模板（构建收敛 2026-08）：15 个标准服务共用。
# 用法：build --build-arg SERVICE=<name>，compose 里 build.dockerfile=Dockerfile
#   + args.SERVICE=<name>。monitor（双二进制）与 system（snapshot）保留各自 Dockerfile。
# fast-deploy 快路径：--build-arg MAIN_SRC=prebuilt 时产物直接取本机交叉编译的
# <SERVICE>/.bin/main，BuildKit 只构建被引用的阶段，容器内编译整段跳过。
ARG SERVICE
ARG MAIN_SRC=builder

FROM golang:1.26-alpine AS builder

ARG SERVICE
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

# 依赖下载层只拷 go.mod/go.sum，改源码不打穿模块缓存。
COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o main ./${SERVICE}/cmd/main.go

# 快路径产物（仅 MAIN_SRC=prebuilt 时被引用）。
FROM alpine:3.20 AS prebuilt
ARG SERVICE
COPY ${SERVICE}/.bin/main /app/main

FROM ${MAIN_SRC} AS main-src

FROM alpine:3.20

WORKDIR /app

# tzdata is required: the libpq DSN pins TimeZone=Asia/Shanghai and pgx
# resolves it via the local zoneinfo database.
# uploads 必须预建：compose 把 go_admin_kit_uploads 命名卷挂到 /app/uploads，
# 卷首次初始化继承镜像挂载点的 owner——缺这步则卷归 root:root，app 用户
# 上传时 mkdir uploads/<yyyy> 直接 permission denied（集成 smoke 实测铁证）。
RUN apk add --no-cache tzdata wget \
    && addgroup -S app && adduser -S app -G app \
    && mkdir -p ./logs ./uploads \
    && chown -R app:app /app

COPY --from=main-src --chown=app:app /app/main .

USER app

CMD ["./main"]
