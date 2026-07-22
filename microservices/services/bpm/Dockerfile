FROM golang:1.26-alpine AS builder
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn
COPY go.mod go.sum ./
RUN go mod download && go mod tidy
COPY . .
RUN go build -trimpath -o main ./cmd/main.go

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache tzdata wget \
    && addgroup -S app && adduser -S app -G app \
    && chown -R app:app /app
COPY --from=builder --chown=app:app /app/main .
USER app
EXPOSE 8096
CMD ["./main"]
