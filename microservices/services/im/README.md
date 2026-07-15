# im-service（M1 + M2 + M3）

自研 IM：访客会话、坐席台、WebSocket、网页埋码 Widget，以及 **技能组 / 排队 / 转接**。

## 能力

| 阶段 | 内容 |
|------|------|
| M1 | 会话/文本消息 REST、WS、访客 H5、坐席台 `/im/desk` |
| M2 | `widget.js` + iframe、`parent_origin` 白名单、站点配置与埋码片段 |
| M3 | 技能组、坐席 presence、排队分配（round_robin / least_load / manual）、转接、关闭原因 |

### M3 要点

| 能力 | 说明 |
|------|------|
| 技能组 | `/im/skills` 管理；表 `im_skill_groups` / `im_agent_skills` |
| 坐席状态 | `online` / `busy` / `offline`（PG 表 `im_agent_presence`） |
| 自动分配 | 访客建会话后，对启用策略的技能组尝试分配在线坐席 |
| 转接 | 指定坐席，或退回排队（可改技能组） |
| 队列视图 | 坐席台 scope：`all` / `mine` / `queue` |

种子数据：技能组 `default`（默认客服组）+ 演示站点 `app_key=demo`。

## 埋码接入

```html
<script
  src="https://你的网关/im/widget/widget.js"
  data-app-key="demo"
  async></script>
```

- 演示页：`/im/widget/demo`
- 站点配置（需登录）：管理台 `/im/sites`
- 默认演示 `app_key=demo`（首次启动自动种子）

请把客户站 `Origin` 加入站点「允许来源」，否则 session 会被拒绝。

## 本地运行

```bash
export DB_HOST=127.0.0.1 DB_PASSWORD=123456 JWT_SECRET=local-dev-secret-change-me-32-chars
go run ./cmd/main.go
```

Compose 服务 `im-service:8088`，Traefik 前缀 `/api/v1/im`、`/im/`。

设计文档：`docs/design/im-service.md`。
