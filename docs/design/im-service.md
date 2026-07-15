# IM / 智能客服接入设计（自研）

> 状态：设计草案（先文档，后实现）  
> 范围：`microservices` 产品线新增 `im-service` + 管理台坐席模块 + **网页埋码 Widget**  
> 原则：完全自研消息面；与 identity/auth/file/ai 解耦协作；单体线不实现完整 IM。

相关文档：

- 扩展总览：[`../EXPANSION_PLAN.md`](../EXPANSION_PLAN.md)
- 产品线边界：[`../PRODUCT_LINES.md`](../PRODUCT_LINES.md)
- FreeSWITCH（独立仓，后对接）：[`freeswitch-cc.md`](freeswitch-cc.md)

---

## 1. 目标与非目标

### 1.1 目标

1. 支持 **访客 ↔ 坐席** 实时会话（文本 + 图片/文件）。
2. 支持 **网页埋码接线**（客户站点嵌入 `widget.js` 即可咨询）。
3. 管理台提供 **坐席工作台**（会话列表、聊天、转接基础能力）。
4. 可挂 **AI 预答**（可选调用现有 `ai-service`），支持转人工。
5. 消息、会话落库可审计；权限走现有 RBAC。

### 1.2 非目标（本阶段不做）

- 完整社交 IM（朋友圈、大规模群、音视频通话）。
- 端到端加密、阅后即焚。
- 替代企业微信/微信客服开放平台全能力。
- 在 `monolith/` 内嵌消息集群。
- FreeSWITCH 话音（见独立呼叫文档）。

### 1.3 成功标准（MVP）

| 场景 | 通过标准 |
|------|----------|
| 埋码 | 第三方页面加载 widget 后可发消息 |
| 坐席 | 管理台可见排队/会话并回复 |
| 历史 | 刷新后双方仍能拉到历史消息 |
| 鉴权 | 访客 guest JWT 不能读写他人会话 |
| 文件 | 图片经 file-service 上传后可展示 |

---

## 2. 总体架构

```text
┌──────────────────┐     widget.js + iframe      ┌─────────────────────┐
│  客户业务网站     │ ──────────────────────────► │  静态资源 / CDN      │
└──────────────────┘                              │  widget.js           │
                                                  │  chat-iframe.html    │
                                                  └──────────┬──────────┘
                                                             │ HTTPS / WSS
                                                             ▼
┌──────────────────┐     坐席工作台 SPA           ┌─────────────────────┐
│ microservices/web│ ──────────────────────────► │      Traefik        │
│  /im/desk        │                              │  /im/api/*  REST    │
└──────────────────┘                              │  /im/ws     WebSocket│
                                                  │  ForwardAuth(坐席)  │
                                                  └──────────┬──────────┘
                                                             │
                         ┌───────────────────────────────────┼───────────────────────────┐
                         ▼                                   ▼                           ▼
                  ┌──────────────┐                   ┌──────────────┐            ┌──────────────┐
                  │  im-service  │◄──── NATS ───────►│ audit/notify │            │ ai-service   │
                  │  会话·消息   │                   │  (可选消费)  │            │ 机器人预答   │
                  │  排队·分配   │                   └──────────────┘            └──────────────┘
                  └──────┬───────┘
                         │
              ┌──────────┼──────────┐
              ▼          ▼          ▼
         PostgreSQL    Redis     file-service
         会话/消息    在线/排队    附件
```

### 2.1 进程与目录（实现时）

```text
microservices/
  services/im/           # 新建
    cmd/
    internal/
    migrations/          # 或统一由 monitor 迁移（二选一，推荐 im 自带域表迁移）
    Dockerfile
  web/
    src/pages/im/        # 坐席台、配置页
  # widget 可放 services/im/web-widget/ 构建产物挂 CDN 或由 im 静态托管
```

Compose：增加 `im-service`，Traefik 标签示例：

- `PathPrefix(/api/v1/im)` → im-service（priority 高）
- `PathPrefix(/im/ws)` → im-service
- `PathPrefix(/im/widget)` → 静态或 im-service

---

## 3. 核心领域模型

### 3.1 概念

| 概念 | 说明 |
|------|------|
| **Channel（通道）** | `web_widget` / `h5` / `miniprogram` / `agent_internal` |
| **Visitor（访客）** | 匿名 guest 或已绑定的正式用户 |
| **Agent（坐席）** | 后台用户，具备 `im:agent:*` 权限 |
| **Conversation（会话）** | 一次咨询线程，含状态机 |
| **Message（消息）** | 会话内有序消息 |
| **Inbox / Queue** | 待分配队列（按技能组） |
| **SkillGroup（技能组）** | 分配路由用 |

### 3.2 会话状态机

```text
                  AI 开启
created ──────► bot_serving ──────► queued ──────► assigned ──────► closed
   │                 │                  │              │              ▲
   └─────────────────┴──────────────────┴──────────────┴── 访客结束 / 超时 / 坐席关闭
```

| 状态 | 含义 |
|------|------|
| `created` | 已建会话，尚未分配 |
| `bot_serving` | 机器人接待中 |
| `queued` | 转人工排队 |
| `assigned` | 已分配坐席 |
| `closed` | 结束（可评价） |

允许的关键迁移：

- `created` → `bot_serving` | `queued` | `assigned`
- `bot_serving` → `queued` | `assigned` | `closed`
- `queued` → `assigned` | `closed`
- `assigned` → `queued`（转接）| `closed`

---

## 4. 数据表草案（PostgreSQL）

> 表前缀建议 `im_`。以下为 MVP 必要表，可按迭代加字段。

### 4.1 `im_sites`（埋码站点 / App）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | bigserial PK | |
| app_key | varchar(64) unique | 公开 appId，widget 使用 |
| app_secret | varchar(128) | 服务端校验用（不落前端） |
| name | varchar(128) | 站点名称 |
| allowed_origins | jsonb | 域名白名单 `["https://a.com"]` |
| welcome_text | text | 欢迎语 |
| offline_text | text | 非工作时间文案 |
| work_hours | jsonb | 工作时间配置 |
| bot_enabled | bool | 是否先机器人 |
| theme | jsonb | 颜色、位置、标题 |
| status | smallint | 1 启用 0 停用 |
| created_at / updated_at | timestamptz | |

### 4.2 `im_visitors`

| 字段 | 类型 | 说明 |
|------|------|------|
| id | bigserial PK | |
| site_id | bigint FK | |
| guest_key | varchar(64) | 浏览器侧 guest_id |
| user_id | bigint null | 绑定正式用户（identity） |
| display_name | varchar(128) | |
| meta | jsonb | UA、首次落地页、自定义属性 |
| last_seen_at | timestamptz | |
| created_at | timestamptz | |

唯一：`(site_id, guest_key)`。

### 4.3 `im_skill_groups` / `im_agent_skills`

| 表 | 要点 |
|----|------|
| im_skill_groups | id, name, code, strategy(`round_robin`/`least_load`), status |
| im_agent_skills | agent_user_id, skill_group_id, max_concurrent, status |

坐席即 `users.id`，不另建用户体系。

### 4.4 `im_conversations`

| 字段 | 类型 | 说明 |
|------|------|------|
| id | bigserial PK | |
| public_id | uuid unique | 对外暴露，防扫库 |
| site_id | bigint | |
| channel | varchar(32) | web_widget / h5 / … |
| visitor_id | bigint | |
| agent_user_id | bigint null | 当前坐席 |
| skill_group_id | bigint null | |
| status | varchar(32) | 状态机 |
| subject | varchar(256) null | |
| context | jsonb | 商品ID、订单号、当前 URL 等 |
| queued_at / assigned_at / closed_at | timestamptz null | |
| close_reason | varchar(64) null | visitor/agent/timeout/system |
| last_message_at | timestamptz | |
| last_message_preview | varchar(256) | |
| created_at / updated_at | timestamptz | |

索引：`(status, skill_group_id, queued_at)`、`(agent_user_id, status)`、`(visitor_id, status)`。

### 4.5 `im_messages`

| 字段 | 类型 | 说明 |
|------|------|------|
| id | bigserial PK | |
| conversation_id | bigint | |
| client_msg_id | varchar(64) null | 客户端幂等 |
| sender_type | varchar(16) | visitor / agent / bot / system |
| sender_id | bigint null | visitor_id 或 user_id |
| msg_type | varchar(32) | text / image / file / card / event |
| content | jsonb | 见消息体规范 |
| seq | bigint | 会话内单调递增 |
| created_at | timestamptz | |

唯一：`(conversation_id, client_msg_id)`（client_msg_id 非空时）。  
索引：`(conversation_id, seq)`。

### 4.6 `im_conversation_events`（可选）

转接、分配、关闭等系统事件，便于审计与报表（也可写入 messages 且 `msg_type=event`）。

### 4.7 `im_agent_presence`（可用 Redis 为主，PG 兜底）

在线、示忙、当前会话数；高频写建议 **Redis**，PG 仅存偏好配置。

---

## 5. 消息体规范（content JSON）

### 5.1 text

```json
{ "text": "你好，我想咨询订单" }
```

### 5.2 image / file

```json
{
  "file_id": 12345,
  "url": "/uploads/...",
  "name": "a.png",
  "size": 102400,
  "mime": "image/png"
}
```

文件先走 `file-service` 上传拿 `file_id`，再发 IM 消息。

### 5.3 card（扩展）

```json
{
  "card_type": "order",
  "title": "订单 2026...",
  "fields": [{ "label": "状态", "value": "已发货" }],
  "link": "https://..."
}
```

### 5.4 event

```json
{ "event": "assigned", "payload": { "agent_user_id": 1, "agent_name": "客服小王" } }
```

---

## 6. REST API 草案

Base：`/api/v1/im`  
鉴权：

- **坐席**：网关 ForwardAuth + Bearer（与现网一致）
- **访客**：`Authorization: Bearer <guest_jwt>` 或 Header `X-IM-Guest-Token`

### 6.1 站点 / Widget 配置（部分公开）

| 方法 | 路径 | 说明 | 鉴权 |
|------|------|------|------|
| GET | `/widget/config?app_key=` | 主题、欢迎语、是否在线（无 secret） | 公开 + Origin 校验 |
| POST | `/visitor/session` | 创建/恢复访客，签发 guest_jwt | app_key + 可选签名 |
| POST | `/visitor/bind` | 将 guest 绑定正式用户 | guest + 站点签名 |

### 6.2 访客会话

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/conversations` | 创建会话（带 context） |
| GET | `/conversations/current` | 当前进行中会话 |
| GET | `/conversations/{public_id}/messages` | 历史消息（cursor/seq） |
| POST | `/conversations/{public_id}/messages` | 发消息（HTTP 兜底；优先 WS） |
| POST | `/conversations/{public_id}/close` | 访客结束 |
| POST | `/conversations/{public_id}/transfer_human` | 机器人转人工 |

### 6.3 坐席

| 方法 | 路径 | 说明 | 权限示例 |
|------|------|------|----------|
| GET | `/agent/me` | 坐席状态、技能组 | `im:agent:access` |
| PUT | `/agent/presence` | online/busy/offline | `im:agent:access` |
| GET | `/agent/conversations` | 我的会话 / 队列 | `im:agent:access` |
| POST | `/agent/conversations/{id}/accept` | 从队列接入 | `im:agent:access` |
| POST | `/agent/conversations/{id}/messages` | 回复 | `im:agent:access` |
| POST | `/agent/conversations/{id}/transfer` | 转接 | `im:agent:transfer` |
| POST | `/agent/conversations/{id}/close` | 关闭 | `im:agent:access` |
| GET | `/admin/sites` CRUD | 埋码站点配置 | `im:site:manage` |
| GET | `/admin/skill-groups` CRUD | 技能组 | `im:skill:manage` |

### 6.4 统一响应

与现有脚手架对齐：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

错误码建议：`IM_FORBIDDEN` / `IM_NOT_IN_CONVERSATION` / `IM_ORIGIN_DENIED` / `IM_RATE_LIMITED`。

---

## 7. WebSocket 协议草案

### 7.1 连接

```text
WSS /im/ws?token=<guest_jwt|agent_jwt>
```

或连接后首帧认证：

```json
{ "type": "auth", "token": "...", "role": "visitor|agent" }
```

服务端：

```json
{ "type": "auth_ok", "conn_id": "..." }
{ "type": "auth_fail", "message": "..." }
```

### 7.2 客户端 → 服务端

| type | 说明 | payload |
|------|------|---------|
| `ping` | 心跳 | `{}` |
| `message.send` | 发消息 | `conversation_public_id, client_msg_id, msg_type, content` |
| `message.read` | 已读到 seq | `conversation_public_id, seq` |
| `typing` | 输入中 | `conversation_public_id, on: true/false` |
| `presence.set` | 坐席状态 | `status` |
| `conversation.subscribe` | 坐席订阅队列变更 | `{}` |

### 7.3 服务端 → 客户端

| type | 说明 |
|------|------|
| `pong` | 心跳响应 |
| `message.new` | 新消息（含完整 message + seq） |
| `message.ack` | 对 client_msg_id 的确认 |
| `conversation.updated` | 状态/坐席变更 |
| `queue.updated` | 坐席队列长度变化 |
| `typing` | 对方输入中 |
| `error` | 业务错误 |

### 7.4 示例：发送

客户端：

```json
{
  "type": "message.send",
  "request_id": "r1",
  "payload": {
    "conversation_public_id": "uuid",
    "client_msg_id": "c-uuid",
    "msg_type": "text",
    "content": { "text": "你好" }
  }
}
```

服务端：

```json
{
  "type": "message.ack",
  "request_id": "r1",
  "payload": { "client_msg_id": "c-uuid", "seq": 12, "id": 9001 }
}
```

并向会话成员广播 `message.new`。

### 7.5 多实例与在线路由

- 连接元数据：`conn_id → node_id` 存 Redis。  
- 消息投递：写 PG 成功后 publish Redis/NATS `im.conv.{id}`，各节点对本机连接 fan-out。  
- 水平扩展不要求粘性 Session，但 WebSocket 升级需网关支持。

---

## 8. 网页埋码（Widget）设计

### 8.1 接入方式

```html
<script
  src="https://im.example.com/widget.js"
  data-app-key="site_public_key"
  data-user-id=""
  data-user-sign=""
  data-extra='{"order_id":"2026"}'
  async>
</script>
```

可选 JS API：

```js
window.GoAdminIM.open()
window.GoAdminIM.close()
window.GoAdminIM.setContext({ order_id: '2026' })
window.GoAdminIM.identify({ userId: 'u1', sign: '...' })
```

### 8.2 安全

| 项 | 策略 |
|----|------|
| 域名 | `allowed_origins` 严格校验 Origin / Referer |
| Secret | 仅服务端；前端只有 app_key |
| 登录用户 | `sign = HMAC_SHA256(app_secret, userId + ts)`，短时有效 |
| 限流 | 按 IP / guest_key / app_key |
| 隔离 | 聊天 UI 用 **iframe** 或 Shadow DOM，避免 CSS 污染 |

### 8.3 加载流程

```text
1. widget.js 读取 data-app-key
2. GET /widget/config
3. 校验当前页面 origin 是否允许（服务端也校验）
4. 渲染悬浮球
5. 用户点击 → 打开 iframe → POST /visitor/session → 拿 guest_jwt
6. 建立 WSS /im/ws
7. POST /conversations（若无进行中会话）
8. 收发 message.*
```

### 8.4 配置项（管理台可改）

- 主题色、标题、位置（左/右下）  
- 欢迎语、离线文案、工作时间  
- 是否启用机器人、默认技能组  
- 自动弹出规则（停留秒数，可选）

---

## 9. 与 AI 协同

| 步骤 | 说明 |
|------|------|
| 1 | 会话进入 `bot_serving`，访客消息转发 `ai-service` |
| 2 | bot 回复 `sender_type=bot` 写入 messages |
| 3 | 置信度低 / 用户点「转人工」→ `queued` |
| 4 | 分配坐席时推送上下文（最近 N 条 + context JSON） |
| 5 | 关闭时可异步生成会话小结（audit/ai） |

失败降级：AI 不可用则直接 `queued` 或提示留言。

---

## 10. 权限码与菜单（草案）

```text
im:agent:access      # 进入坐席台
im:agent:transfer    # 转接
im:agent:monitor     # 监控他人会话（后期）
im:site:manage       # 埋码站点
im:skill:manage      # 技能组
im:report:view       # 报表（后期）
```

菜单示例：智能客服 / 坐席工作台、站点配置、技能组。

---

## 11. 事件（NATS，可选）

| Subject | 何时 | 用途 |
|---------|------|------|
| `im.conversation.created` | 建会话 | 统计 |
| `im.conversation.assigned` | 分配 | 通知坐席 |
| `im.conversation.closed` | 关闭 | 质检、小结 |
| `im.message.created` | 新消息 | 审计抽检（注意脱敏） |

---

## 12. 实现分期

### M1 · 内核（约 1～2 周量级，视人力）

- [ ] 表结构 migration  
- [ ] 会话 + 文本消息 REST  
- [ ] WebSocket 收发 + ack  
- [ ] 坐席台最小 UI（列表 + 聊天）  
- [ ] 访客 H5 页（非埋码）  

### M2 · 埋码

- [ ] widget.js + iframe  
- [ ] guest session、Origin 白名单  
- [ ] 站点管理 CRUD  
- [ ] 图片/文件消息  

### M3 · 客服

- [ ] 排队、技能组、分配策略  
- [ ] 转接、关闭原因  
- [ ] 坐席 presence  

### M4 · 智能

- [ ] 对接 ai-service  
- [ ] 转人工、会话小结  

---

## 13. 测试要点

| 类型 | 用例 |
|------|------|
| 单测 | 状态机迁移、分配策略、幂等 client_msg_id |
| 集成 | WS 双端收发、历史分页 |
| 安全 | guest 越权、Origin 伪造、过期 token |
| 埋码 | 跨域、iframe 通信、限流 |
| 回归 | 不影响现有 auth/gateway smoke |

---

## 14. 开放问题（实现前拍板）

1. 域表 migration 放 `im` 自管还是并入 `monitor` 统一链？**建议 im 自管。**  
2. 消息是否软删除 / 保留时长（合规）？  
3. 是否第一期就上多租户 `tenant_id` 字段预留？  
4. Widget 域名用独立子域还是与网关同域路径？**建议独立子域方便 CDN。**  

---

## 15. 修订记录

| 日期 | 说明 |
|------|------|
| 2026-07-16 | 初稿：模型、API、WS、埋码、分期 |
