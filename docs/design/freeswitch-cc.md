# FreeSWITCH 呼叫中心增强设计（独立项目）

> 状态：设计草案；**FS-M1 骨架已在独立仓落地**：`../go-freeswitch-cc`（与 go-admin-kit 同级目录）  
> 定位：**媒体面独立仓库**，基于 FreeSWITCH 开源增强；  
> 与 **Go Admin Kit** 的关系：本仓只做 B 端运营台 + 可选 `cc-adapter`，**不内嵌 FS 二进制**。

相关文档：

- 扩展总览：[`../EXPANSION_PLAN.md`](../EXPANSION_PLAN.md)
- IM / 网页埋码：[`im-service.md`](im-service.md)
- 产品线边界：[`../PRODUCT_LINES.md`](../PRODUCT_LINES.md)

---

## 1. 为什么必须独立项目

| 维度 | FreeSWITCH 增强仓 | Go Admin Kit |
|------|-------------------|--------------|
| 技术栈 | C/拨号计划/Lua/ESL、实时媒体 | Go / React 管理台 |
| 运行形态 | 媒体集群、中继、录音盘 | API + 控制台 |
| 发布节奏 | 中继/编解码/HA 变更频繁 | 业务功能迭代 |
| 故障域 | 媒体故障不应拖垮后台 | 后台故障仍可保留录音落盘策略 |

**结论**：呼叫中心 = **两个产品** 协同，而不是一个 monorepo 里的一个文件夹。

建议独立仓名（示例，可改）：

- `go-freeswitch-cc` / `fs-admin-kit` / `your-org-freeswitch-cc`

---

## 2. 目标与非目标

### 2.1 独立仓目标

1. 基于 **FreeSWITCH** 提供可部署的呼叫媒体能力（呼入/呼出、IVR、队列、录音）。  
2. 提供 **稳定对接面**：HTTP API + ESL/事件 Webhook，供 Go Admin Kit 消费。  
3. 支持 **开源增强**：模块化拨号计划、配置即代码、Docker Compose 一键体验。  
4. 坐席分机、技能队列与话单（CDR）可查询、可回放索引。

### 2.2 Go Admin Kit 侧目标

1. 坐席 / 技能组 / 排班等 **业务配置** UI。  
2. 话单列表、录音回放权限、质检、报表。  
3. 与客户视图、IM 会话、工单 **汇聚**（同一 `customer_id`）。  
4. 可选 `cc-adapter` 微服务：翻译 FS 事件 → 本仓领域事件 / 入库。

### 2.3 非目标（首期）

- 自研 SIP 协议栈替代 FreeSWITCH。  
- 在本仓编译/打包 `libfreeswitch`。  
- 一次做全全功能 ACD/预测外呼/质检大模型（可分期）。  
- 单体线内嵌呼叫媒体。

---

## 3. 总体拓扑

```text
                    ┌─────────────────────────────────────┐
                    │         Go Admin Kit 仓              │
                    │  React 控制台 · RBAC · 话单/质检 UI  │
                    │  cc-adapter（可选微服务）            │
                    └──────────────▲──────────────────────┘
                                   │ HTTPS API / Webhook / NATS
                                   │ 配置下发 · 话单回传 · 坐席状态
                    ┌──────────────┴──────────────────────┐
                    │     FreeSWITCH 增强仓（独立）         │
                    │  ┌─────────────┐  ┌──────────────┐  │
                    │  │ FreeSWITCH  │  │ control-api  │  │
                    │  │ 媒体/信令   │◄─►│ Go/ESL 守护  │  │
                    │  └──────┬──────┘  └──────────────┘  │
                    │         │ 录音文件 → MinIO/本地盘     │
                    │  ┌──────▼──────┐                    │
                    │  │ Postgres    │  CDR / 分机 / 队列  │
                    │  └─────────────┘                    │
                    └─────────────────────────────────────┘
                                   ▲
                                   │ SIP / WebRTC
                    ┌──────────────┴──────────────┐
                    │  中继/运营商 · 坐席软电话     │
                    │  访客 PSTN / WebRTC 进线     │
                    └─────────────────────────────┘
```

---

## 4. 独立仓建议目录结构

```text
freeswitch-cc/                    # 独立 Git 仓库
├── README.md
├── docker-compose.yml            # FS + 控制面 + DB + 可选 Redis
├── freeswitch/
│   ├── conf/                     # 拨号计划、sip_profiles、autoload
│   ├── scripts/                  # Lua / 拨号逻辑
│   └── Dockerfile
├── control-api/                  # 推荐 Go：封装 ESL + 对外 HTTP
│   ├── cmd/
│   ├── internal/esl/
│   ├── internal/api/
│   └── Dockerfile
├── deploy/
│   ├── systemd/ 或 k8s/
│   └── env.example
└── docs/
    ├── architecture.md
    ├── dialplan.md
    └── integration-go-admin-kit.md
```

---

## 5. FreeSWITCH 能力分期

### P0 · 可打电话的最小集

| 能力 | 说明 |
|------|------|
| 分机注册 | 内线 SIP/WebRTC 坐席 |
| 呼入 | 中继 DID → IVR 或队列 |
| 队列 / 技能组 | `mod_callcenter` 或拨号计划模拟 |
| 录音 | 通话录音落盘，路径可索引 |
| CDR | 基础话单入库或写文件再采集 |
| 控制 API | 发起监听、挂断、查询通道（ESL） |

### P1 · 运营增强

| 能力 | 说明 |
|------|------|
| IVR 菜单 | 按键路由到技能组 |
| 示忙/签入 | 与 callcenter agent 状态同步 |
| 录音上传 | 异步上传 MinIO，回写 URL |
| Webhook | 关键事件推送到 Go Admin Kit |
| 安全 | ESL 仅内网、TLS、ACL |

### P2 · 进阶

| 能力 | 说明 |
|------|------|
| 外呼任务 | 预览式外呼（谨慎，合规） |
| 会议 / 三方 | 质检监听、三方通话 |
| HA | 双机、共享存储、注册中心 |
| 多租户中继 | 配置隔离 |

---

## 6. 对接协议（Go Admin Kit ↔ FS 仓）

### 6.1 配置下发（Kit → FS 控制面）

管理台保存后，由 `cc-adapter` 或直接调 FS `control-api`：

| 接口（示例） | 说明 |
|--------------|------|
| `PUT /v1/agents/{ext}` | 分机、密码、技能组 |
| `PUT /v1/queues/{id}` | 队列策略、超时 |
| `POST /v1/reload` | 热加载拨号相关配置（谨慎） |

鉴权：mTLS 或内网 + `X-CC-Token` 共享密钥。

### 6.2 事件回传（FS → Kit）

建议 Webhook POST 到本仓 `cc-adapter`：

| 事件 | 触发 | 载荷要点 |
|------|------|----------|
| `call.ringing` | 振铃 | call_id, direction, caller, callee, queue |
| `call.answered` | 接通 | call_id, agent_ext, answered_at |
| `call.hangup` | 挂断 | call_id, cause, duration |
| `recording.ready` | 录音完成 | call_id, path/url, duration |
| `agent.status` | 坐席状态 | ext, status(logged_out/available/on_call) |
| `queue.stats` | 可选 | waiting, agents |

本仓入库表示例（在 Kit 库，非 FS 库）：

- `cc_calls`：话单  
- `cc_recordings`：录音元数据  
- `cc_agents`：分机与 user_id 映射  

### 6.3 控制命令（Kit → FS）

| 命令 | 用途 |
|------|------|
| `originate` | 点拨（外呼） |
| `uuid_kill` | 强拆 |
| `uuid_record` | 控制录音 |
| `callcenter_config` | 坐席签入签出（若用 mod_callcenter） |

封装在 `control-api`，**禁止**管理台直连 ESL 端口。

### 6.4 身份映射

```text
Go Admin Kit user_id  ←→  SIP 分机号 extension
im visitor/customer   ←→  主叫号码 / 客户 ID（来电弹屏）
```

来电时：`caller_number` → 查客户 → 坐席台弹屏（与 IM 共用客户视图时效果最好）。

---

## 7. 与 IM / 智能客服的关系

```text
同一客户 customer_id
   ├─ IM 会话（web 埋码 / 小程序）
   ├─ 通话记录（FreeSWITCH CDR）
   └─ 工单 ticket
```

- **不要求** IM 与通话共用一条消息通道。  
- **要求** 客户标识可对齐，管理台「客户 360」可聚合。  
- 智能客服：IM 走机器人；电话侧 IVR/ASR 可后期接 AI，仍经事件回写 Kit。

---

## 8. Go Admin Kit 侧实现边界

### 8.1 本仓新增（后期实现时）

| 组件 | 说明 |
|------|------|
| `services/cc-adapter` | 收 Webhook、写话单、转 NATS、鉴权调 FS API |
| `web` 页面 | 话单、录音、坐席分机绑定、队列配置 |
| 权限 | `cc:call:list` / `cc:recording:play` / `cc:agent:manage` |
| 菜单种子 | 「呼叫中心」模块 |

### 8.2 明确不进本仓

- FreeSWITCH 源码/模块/拨号计划主配置  
- RTP 端口段与媒体网卡配置  
- 运营商中继账号的运行时进程  

### 8.3 环境变量（adapter 示例）

```env
CC_FS_API_BASE=https://fs-control.internal
CC_FS_TOKEN=***
CC_WEBHOOK_SECRET=***
CC_RECORDING_PUBLIC_BASE=https://minio...
```

---

## 9. 安全与合规

| 项 | 要求 |
|----|------|
| ESL | 仅绑定内网；强密码；禁止暴露公网 |
| 录音 | 权限控制播放/下载；操作进审计日志 |
| 中继账号 | 密钥进密钥管理，不进前端 |
| 合规外呼 | 频次限制、退订、本地法规（业务层强制） |
| 日志 | 勿打印 SIP 明文密码、完整录音路径可对普通坐席脱敏 |

---

## 10. 部署形态建议

### 开发

```bash
# 在 freeswitch-cc 仓
docker compose up -d
# FS + control-api + postgres
```

### 生产

- FS 与业务 API **分机器或分网段**  
- 录音盘独立、定期备份  
- control-api 与 Kit 之间专线 / 内网  
- 监控：通道数、CPS、注册数、磁盘  

---

## 11. 仓库落地清单（建仓时）

### FreeSWITCH 仓

- [x] 基础 Docker 镜像与 `docker-compose`  
- [x] 内线分机注册示例  
- [x] 一条呼入到队列的拨号计划（演示 5000）  
- [x] 录音目录 + control-api CDR 表  
- [x] `control-api`：健康检查、ESL、Webhook 出站、事件入库  
- [x] `docs/integration-go-admin-kit.md` 对接契约  
- [ ] 开源 LICENSE 与模块边界说明（增强哪些、如何配置）

### Go Admin Kit 仓（后置 PR）

- [ ] `cc-adapter` 骨架 + Webhook 验签  
- [ ] 话单表 migration  
- [ ] 话单列表 / 录音播放页  
- [ ] 用户绑定分机号  
- [ ] 权限与菜单  

---

## 12. 里程碑建议

| 里程碑 | 交付 |
|--------|------|
| FS-M1 | 独立仓能内线互通 + 录音 + CDR |
| FS-M2 | Webhook 打到测试 adapter，Kit 可见话单 |
| FS-M3 | 坐席签入与队列、管理台状态一致 |
| FS-M4 | 来电弹屏（客户视图）+ 质检播放权限 |
| FS-M5 | 生产 HA 与中继文档 |

---

## 13. 开放问题

1. 坐席软电话用 WebRTC（浏览器）还是硬话机/第三方 SIP 话机？  
2. 录音存 FS 本地还是统一 MinIO（与 file-service 一致）？  
3. 是否第一期就上 `mod_callcenter`？  
4. 多租户：一套 FS 多租户隔离，还是租户独立 FS 实例？  
5. 独立仓是否公开开源，与 Kit 同品牌还是另品牌？  

---

## 14. 和 IM 文档的衔接

| 项目 | IM（本仓服务） | 呼叫（独立仓） |
|------|----------------|----------------|
| 实时通道 | WebSocket 消息 | SIP/RTP 媒体 |
| 访客入口 | 网页埋码 Widget | PSTN / WebRTC 进线 |
| 管理台 | 同一 React 控制台不同菜单 | 同左 |
| 客户 | 建议统一 customer 模型 | 同左 |
| 实现顺序 | 可先于呼叫 | 媒体复杂，建议 IM 闭环后再对接 |

推荐节奏：**先 IM 埋码闭环 → 并行起 FS 独立仓 P0 → 再写 Kit adapter**。

---

## 15. 修订记录

| 日期 | 说明 |
|------|------|
| 2026-07-16 | 初稿：独立仓边界、拓扑、对接事件、分期 |
