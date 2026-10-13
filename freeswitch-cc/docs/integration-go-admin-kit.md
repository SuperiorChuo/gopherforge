# 与 Go Admin Kit 对接契约

本目录（媒体面 `freeswitch-cc/`）→ monorepo 管理面（`microservices/` + web）。

## 1. 鉴权

| 方向 | 方式 |
|------|------|
| Kit → control-api | Header `X-CC-Token: <CC_API_TOKEN>` |
| control-api → Kit Webhook | Header `X-CC-Signature: sha256=<hmac>`，Body 原始 JSON |

HMAC：`HMAC_SHA256(CC_WEBHOOK_SECRET, raw_body)`，hex 编码。

## 2. Webhook 事件（FS → Kit）

统一 envelope：

```json
{
  "event": "call.hangup",
  "source": "go-freeswitch-cc",
  "sent_at": "2026-07-16T00:00:00Z",
  "payload": {
    "call_id": "uuid",
    "direction": "inbound",
    "caller": "13800000000",
    "callee": "5000",
    "agent_ext": "1000",
    "queue": "demo",
    "duration_sec": 42,
    "recording": "/recordings/xxx.wav"
  }
}
```

| event | 含义 |
|-------|------|
| `call.test` | 联调探测 |
| `call.ringing` | 振铃 |
| `call.answered` | 接通 |
| `call.hangup` | 挂断 |
| `recording.ready` | 录音就绪 |
| `agent.status` | 坐席状态（FS-M3） |

Kit 侧建议路径（后续实现）：

```text
POST /api/v1/cc/webhook
```

由 `cc-adapter` 验签、入库 `cc_calls`，再可选发 NATS。

## 3. Kit → control-api（配置/查询）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/extensions` | 分机清单 |
| GET | `/v1/cdr` | 本仓 CDR |
| GET | `/v1/esl/status` | FS 状态 |
| POST | `/v1/webhooks/test` | 打测试事件 |

## 4. 分机与用户映射

| FreeSWITCH | Go Admin Kit |
|------------|----------------|
| SIP ext `1000` | `users.id` 绑定字段 `sip_extension`（待加） |
| 主叫号码 | 客户表 / visitor |

## 5. 联调步骤（FS-M2）

1. `cd freeswitch-cc && docker compose up -d`  
2. Kit 起 `cc-adapter` 接收 Webhook  
3. `.env` 设置 `CC_WEBHOOK_URL`  
4. `curl -H "X-CC-Token: ..." -X POST localhost:8090/v1/webhooks/test`  
5. 软电话注册 1000/1001 互拨，确认录音目录有文件  

## 6. 网络注意

- **macOS Docker Desktop**：推荐 `docker compose --profile bridge up -d` 使用 `freeswitch-bridge`。  
- **Linux**：可用默认 `freeswitch`（`network_mode: host`）降低 RTP 问题。  
- control-api 默认 `CC_ESL_HOST=host.docker.internal` 连 host 网络上的 FS。
