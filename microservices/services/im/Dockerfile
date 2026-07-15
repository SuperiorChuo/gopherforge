FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -o main ./cmd/main.go

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache tzdata wget \
    && addgroup -S app && adduser -S app -G app \
    && mkdir -p ./logs && chown -R app:app /app
COPY --from=builder --chown=app:app /app/main .
COPY --chown=app:app ./web-visitor ./web-visitor
USER app
EXPOSE 8088
CMD ["./main"]
