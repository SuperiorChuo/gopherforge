# Build backend with Go 1.26.
FROM golang:1.26-alpine AS builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

COPY go.mod go.sum ./

RUN go mod download && go mod tidy

COPY . .

RUN go build -trimpath -o main ./cmd/main.go \
    && go build -trimpath -o migrate ./cmd/migrate

# Run the backend on a pinned Alpine image as an unprivileged user.
FROM alpine:3.20

WORKDIR /app

# tzdata is required: the libpq DSN pins TimeZone=Asia/Shanghai and pgx
# resolves it via the local zoneinfo database.
RUN apk add --no-cache tzdata \
    && addgroup -S app && adduser -S app -G app && mkdir -p ./logs && chown -R app:app /app

COPY --from=builder --chown=app:app /app/main .
COPY --from=builder --chown=app:app /app/migrate .
COPY --chown=app:app ./configs ./configs
COPY --chown=app:app ./migrations ./migrations
COPY --chown=app:app ./resources ./resources
# 仓库/rsync 常排除 configs/config.yaml；compose 靠环境变量覆盖密钥
RUN if [ ! -f ./configs/config.yaml ]; then cp ./configs/config.example.yaml ./configs/config.yaml; fi \
    && chown app:app ./configs/config.yaml

USER app

EXPOSE 8081

CMD ["sh", "-c", "./migrate -config ./configs/config.yaml -dir ./migrations up && ./main"]
