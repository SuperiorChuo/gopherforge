# Build system-service with Go 1.26.
FROM golang:1.26-alpine AS builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

# 构建上下文是 microservices：shared 拷到 /shared，命中 go.mod 的 replace ../shared（/app/../shared）
COPY services/shared/ /shared/
COPY services/system/go.mod services/system/go.sum ./

RUN go mod download && go mod tidy

COPY services/system/ .

RUN go build -trimpath -o main ./cmd/main.go

# 预检与 ZIP 只需要这些接入源文件；快照在构建时固化，不挂载宿主仓库。
COPY services/system/internal/api/routes.go /repository-snapshot/microservices/services/system/internal/api/routes.go
COPY web/src/router/index.tsx /repository-snapshot/microservices/web/src/router/index.tsx
COPY web/src/layouts/MainLayout.tsx /repository-snapshot/microservices/web/src/layouts/MainLayout.tsx
COPY services/system/internal/service/system/menu_seed.go /repository-snapshot/microservices/services/system/internal/service/system/menu_seed.go
COPY services/monitor/migrations/ /repository-snapshot/microservices/services/monitor/migrations/

# Run the system-service on a pinned Alpine image as an unprivileged user.
# Configuration is environment-only; migrations are owned by the monolith.
FROM alpine:3.20

WORKDIR /app

# tzdata is required: the libpq DSN pins TimeZone=Asia/Shanghai and pgx
# resolves it via the local zoneinfo database.
RUN apk add --no-cache tzdata \
    && addgroup -S app && adduser -S app -G app && mkdir -p ./logs && chown -R app:app /app

COPY --from=builder --chown=app:app /app/main .
COPY --from=builder /repository-snapshot /app/repository-snapshot

ENV CODEGEN_REPO_ROOT=/app/repository-snapshot \
    CODEGEN_WRITE_ENABLED=false

USER app

EXPOSE 8084

CMD ["./main"]
