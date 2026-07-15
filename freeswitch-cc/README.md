# ☎️ Go FreeSWITCH CC

基于 **FreeSWITCH** 的呼叫中心媒体子系统（**本 monorepo 第三产品线**，一人维护；进程/镜像仍独立部署）。

归属 **Go Admin Kit** monorepo 目录 `freeswitch-cc/`（非第二个 Git 远程；与 `microservices/`、`monolith/` 并列）。

| 本目录 | 中台（microservices / web） |
|--------|-----------------------------|
| SIP/RTP、拨号计划、录音、CDR、ESL | 坐席台 UI、RBAC、话单展示、质检 |
| 媒体进程独立部署、可单独升级 | 微服务经 control-api **控制** FS |

中台设计说明：[`docs/design/freeswitch-cc.md`](../docs/design/freeswitch-cc.md)

---

## ✨ FS-M1 当前能力

- [x] Docker Compose：FreeSWITCH + control-api + PostgreSQL  
- [x] 内线分机示例：`1000`–`1003`（密码见下文）  
- [x] 本地互拨拨号计划  
- [x] 演示「队列」分机 `5000`（轮询 bridge 坐席）  
- [x] 录音目录挂载  
- [x] `control-api`：健康检查、分机列表、ESL 状态、模拟/转发 Webhook、话单写入  
- [x] 与 Go Admin Kit 对接契约文档  

---

## 🚀 快速开始

### 要求

- Docker Desktop  
- 可选：Go 1.22+（本地跑 control-api）  
- SIP 软电话（Linphone / Zoiper 等）做内线测试  

### 启动

```bash
cd freeswitch-cc
cp .env.example .env
docker compose up -d --build
```

| 服务 | 地址 |
|------|------|
| FreeSWITCH SIP | `UDP/TCP 5060` |
| FreeSWITCH ESL | `8021`（仅容器网，映射到宿主机仅调试） |
| control-api | http://localhost:8090 |
| PostgreSQL | `localhost:5434`（避免和 Admin Kit 5432/5433 冲突） |

健康检查：

```bash
curl -s http://localhost:8090/health | jq .
curl -s http://localhost:8090/v1/extensions | jq .
```

### 内线分机（默认）

| 分机 | 密码 | 说明 |
|------|------|------|
| 1000 | 1234 | 坐席 A |
| 1001 | 1234 | 坐席 B |
| 1002 | 1234 | 坐席 C |
| 1003 | 1234 | 坐席 D |
| 5000 | — | 拨打进入演示队列（轮询 1000–1003） |

软电话注册：

- 域/服务器：`你的宿主机 IP` 或 `127.0.0.1`  
- 用户：`1000` 密码：`1234`  
- 传输：UDP  

测试：`1000` 拨 `1001`；或拨 `5000` 走队列演示。

---

## 📂 目录结构

```text
freeswitch-cc/
├── docker-compose.yml
├── .env.example
├── freeswitch/                 # FS 配置与镜像
│   ├── Dockerfile
│   └── conf/
├── control-api/                # Go：ESL 封装 + HTTP + Webhook
│   ├── cmd/main.go
│   └── internal/
├── docs/
│   ├── integration-go-admin-kit.md
│   └── dialplan.md
├── recordings/                 # 录音挂载（gitignore 内容）
└── README.md
```

---

## 🔌 control-api 一览

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 存活 |
| GET | `/ready` | ESL + DB 就绪 |
| GET | `/v1/extensions` | 分机清单（配置） |
| GET | `/v1/esl/status` | ESL `status` 原始输出 |
| POST | `/v1/esl/api` | 执行 ESL API（受 token 保护） |
| GET | `/v1/cdr` | 本地库话单列表 |
| POST | `/v1/webhooks/test` | 向 Admin Kit 地址发测试事件 |
| POST | `/v1/events/ingest` | 接收内部/脚本推送的呼叫事件并转发 |

鉴权：除 `/health` 外，默认要求头：

```http
X-CC-Token: <与 .env 中 CC_API_TOKEN 一致>
```

---

## 🔗 对接 Go Admin Kit

见 [`docs/integration-go-admin-kit.md`](docs/integration-go-admin-kit.md)。

后续在 Admin Kit 增加 `cc-adapter` 接收：

- `call.ringing` / `call.answered` / `call.hangup` / `recording.ready` / `agent.status`

---

## 🗺️ 里程碑

| 编号 | 内容 | 状态 |
|------|------|:----:|
| FS-M1 | Compose + 内线 + 录音目录 + control-api + 对接文档 | ✅ 骨架 |
| FS-M2 | 真实 CHANNEL 事件 → Webhook → Kit 话单 | ⬜ |
| FS-M3 | callcenter 坐席签入与队列状态 | ⬜ |
| FS-M4 | 来电弹屏字段 + 录音 URL | ⬜ |
| FS-M5 | 生产 HA / 中继文档 | ⬜ |

---

## ⚠️ 安全提示

- 默认分机密码 **仅供开发**，生产务必修改。  
- ESL 不要对公网暴露；`8021` 映射仅限本机调试。  
- `CC_API_TOKEN` / Webhook Secret 与 Admin Kit 共用时使用高强度随机串。  

---

## License

MIT（与 Go Admin Kit 对齐，可按组织调整）。
