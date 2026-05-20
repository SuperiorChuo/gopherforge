# 使用 Go 1.26 构建后端
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -trimpath -o main ./cmd/main.go

# 使用轻量 Alpine 运行后端
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/main .
COPY ./configs ./configs
COPY ./resources ./resources

RUN mkdir -p ./logs

EXPOSE 8081

CMD ["./main"]
