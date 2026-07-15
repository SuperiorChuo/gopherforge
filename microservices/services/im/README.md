# im-service（M1 骨架）

自研 IM 最小闭环：访客 H5 + 坐席台 + REST/WebSocket + 自动建表。

## 能力（M1）

- 访客会话签发（guest JWT）
- 创建会话 / 文本消息 / 历史
- WebSocket 收发与 ack
- 坐席列表 / 接入 / 结束
- 演示站点 `app_key=demo` 自动种子

## 本地运行

```bash
# 需 PostgreSQL（可用 microservices compose 只起库）
export DB_HOST=127.0.0.1 DB_PASSWORD=123456 DB_NAME=go_admin_kit
export JWT_SECRET=local-dev-secret-change-me-32-chars
go run ./cmd/main.go
```

- 健康：`GET /api/v1/im/health/ready`
- 访客页：`http://localhost:8088/im/visitor`
- 网关后：`http://localhost:8000/im/visitor`
- 坐席台：管理前端 `/im/desk`（需登录）

## Compose

服务名 `im-service`，端口 `8088`，Traefik：`/api/v1/im`、`/im/*`。

## 设计文档

见仓库 `docs/design/im-service.md`。
