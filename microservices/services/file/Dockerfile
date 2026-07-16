# Build file-service with Go 1.26.
FROM golang:1.26-alpine AS builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

COPY go.mod go.sum ./

RUN go mod download && go mod tidy

COPY . .

RUN go build -trimpath -o main ./cmd/main.go

# Run the file-service on a pinned Alpine image as an unprivileged user.
# Configuration is environment-only; migrations are owned by the monolith.
FROM alpine:3.20

WORKDIR /app

# tzdata is required: the libpq DSN pins TimeZone=Asia/Shanghai and pgx
# resolves it via the local zoneinfo database.
RUN apk add --no-cache tzdata \
    && addgroup -S app && adduser -S app -G app && mkdir -p ./logs ./uploads && chown -R app:app /app

COPY --from=builder --chown=app:app /app/main .

USER app

EXPOSE 8086

CMD ["./main"]
