FROM golang:1.27-alpine AS builder
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -o /control-api ./cmd/main.go

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /control-api /app/control-api
# root 运行：动态分机目录卷由 FS 容器（root）初始化，非 root 写不进；
# 该容器只在内网 compose 网络暴露，生产收紧时改共享 gid + fsGroup
EXPOSE 8090
CMD ["/app/control-api"]
