# im-service（M1～M4）· 实验特性 🧪

> **非生产承诺**：可本地演示与继续开发；API / 表结构可能变。正式版 `v0.1.0` 暂不纳入承诺范围。

自研 IM：访客会话、坐席台、WebSocket、网页埋码、技能组排队，以及 **机器人预答 / 转人工 / 会话小结**。

## 能力

| 阶段 | 内容 |
|------|------|
| M1 | 会话/文本消息 REST、WS、访客 H5、坐席台 `/im/desk` |
| M2 | `widget.js` + iframe、`parent_origin` 白名单、站点配置与埋码片段 |
| M3 | 技能组、坐席 presence、排队分配、转接、关闭原因 |
| M4 | 机器人预答（OpenAI 兼容或本地 stub）、转人工、会话小结 |

### M4 要点

| 能力 | 说明 |
|------|------|
| `bot_serving` | 站点 `bot_enabled=true` 且 `AI_ENABLED=true` 时新建会话先机器人接待 |
| 回复 | 访客消息异步调用 bot；`sender_type=bot` |
| 转人工 | 按钮 / 关键词「转人工」→ `queued` → 技能组自动分配 |
| 小结 | 坐席台「小结」或 `POST .../summary` |
| 降级 | 无 `AI_API_KEY` 时用本地 stub 规则回复；AI 失败提示转人工 |

### 环境变量（AI）

| 变量 | 默认 | 说明 |
|------|------|------|
| `AI_ENABLED` | `true` | 总开关；false 时新建会话直接排队 |
| `AI_BASE_URL` / `OPENAI_BASE_URL` | `https://api.openai.com` | OpenAI 兼容基址 |
| `AI_API_KEY` / `OPENAI_API_KEY` | 空 | 有则走真实模型，无则 stub |
| `AI_MODEL` | `gpt-4o-mini` | 模型名 |
| `AI_SYSTEM_PROMPT` | 内置中文客服提示 | 可被站点 `bot_system_prompt` 覆盖 |
| `AI_TIMEOUT_SEC` | `45` | 调用超时 |

与 `ai-service` 同协议（`/v1/chat/completions`），可共用同一网关/Key；IM 进程内直连，不经 SSE 坐席会话。

## 埋码接入

```html
<script
  src="https://你的网关/im/widget/widget.js"
  data-app-key="demo"
  async></script>
```

- 演示页：`/im/widget/demo.html`
- 站点配置：`/im/sites`（含机器人开关）
- 技能组：`/im/skills`
- 坐席台：`/im/desk`

## 本地运行

```bash
export DB_HOST=127.0.0.1 DB_PASSWORD=123456 JWT_SECRET=local-dev-secret-change-me-32-chars
# 可选真实模型：
# export AI_API_KEY=sk-... AI_BASE_URL=https://api.openai.com AI_MODEL=gpt-4o-mini
go run ./cmd/main.go
```

Compose 服务 `im-service:8088`，Traefik 前缀 `/api/v1/im`、`/im/`。

设计文档：`docs/design/im-service.md`。
