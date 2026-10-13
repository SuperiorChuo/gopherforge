# ☎️ Go FreeSWITCH CC

基于 **FreeSWITCH** 的呼叫中心媒体子系统（**本 monorepo 第三产品线**，一人维护；进程/镜像仍独立部署）。

归属 **Go Admin Kit** monorepo 目录 `freeswitch-cc/`（非第二个 Git 远程；与 `microservices/`、`monolith/` 并列）。

| 本目录 | 中台（microservices / web） |
|--------|-----------------------------|
| SIP/RTP、拨号计划、录音、CDR、ESL | 坐席台 UI、RBAC、话单展示、质检 |
| 媒体进程独立部署、可单独升级 | 微服务经 control-api **控制** FS |

中台接入说明：[`docs/integration-go-admin-kit.md`](docs/integration-go-admin-kit.md)

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
| WebPhone SIP WebSocket | `ws://localhost:5066`（开发）/ `wss://域名:7443`（生产） |
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
| 1000 | mNDhmMVpzZ6p | 坐席 A |
| 1001 | pdE4QKmAdQAJ | 坐席 B |
| 1002 | GmDCK9GyegX8 | 坐席 C |
| 1003 | Ptkku8dbWc5w | 坐席 D |
| 5000 | — | 拨打进入演示队列（轮询 1000–1003） |

软电话注册：

- 域/服务器：`你的宿主机 IP` 或 `127.0.0.1`  
- 用户：`1000` 密码：`mNDhmMVpzZ6p`  
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
| GET | `/metrics` | Prometheus 指标（**鉴权组外**，仅内网抓取） |
| GET | `/v1/extensions` | 分机清单（配置） |
| GET | `/v1/esl/status` | ESL `status` 原始输出 |
| POST | `/v1/esl/api` | 执行 ESL API（受 token 保护） |
| GET | `/v1/cdr` | 本地库话单列表 |
| POST | `/v1/webhooks/test` | 向 Admin Kit 地址发测试事件 |
| POST | `/v1/events/ingest` | 接收内部/脚本推送的呼叫事件并转发 |
| POST | `/v1/fs-xml` | mod_xml_curl 动态目录查询（FS 调用，BasicAuth，见下节） |

鉴权：除 `/health` 外，默认要求头：

```http
X-CC-Token: <与 .env 中 CC_API_TOKEN 一致>
```

> 例外：`/v1/fs-xml` 由 FreeSWITCH 直接调用，走 **BasicAuth**（用户固定 `fs`、
> 口令与 `CC_API_TOKEN` 同源），不走 `X-CC-Token` 头。

### 出局早媒体提示音识别

网关出局腿显式导出 `cc_early_media=1`。control-api 收到
`CHANNEL_PROGRESS_MEDIA` 后在独立 `fscc_early_media` 卷录制有界短样本，接通或
挂机立即取消；样本读入后先删文件，再 HMAC 调用 cc-service 的既有 ASR 热配置。
命中空号、关机/停机、忙线运营商提示语才主动挂断；录音/ASR/HTTP 异常全部放行。

配置：`CC_EARLY_MEDIA_URL`（空为关闭）、`CC_EARLY_MEDIA_SECONDS`（默认 4，
运行时钳制 2–8）、`CC_EARLY_MEDIA_DIR`（容器默认 `/data/early-media`）。
hostnet 部署下，ESL 服务名不可达时会自动回退 `host.docker.internal`。
短样本的 `RECORD_STOP` 不进入正式通话录音链路，ASR 正文不落库、不写日志。

---

## 📇 动态目录（xml_curl）

### 双轨设计：动态优先，文件兜底

分机（SIP 用户）有两条并行的目录来源，FreeSWITCH 优先走动态、失败自动回退文件：

1. **动态（mod_xml_curl）**：FS 每次做 directory 查询（注册鉴权、呼叫定位用户）时，
   HTTP `POST /v1/fs-xml` 实时问 control-api。control-api 按分机号查库，命中即返回
   directory XML（结构与磁盘物化文件一致：`password` param、`user_context` /
   `effective_caller_id_*` 等 variables）。**SIP 密码只在库里，不落盘。**
2. **文件（磁盘物化）**：control-api 仍把动态分机物化到共享卷
   `/etc/freeswitch/directory/default/<ext>.xml`（启动自愈全量重写）。静态模板分机
   （1000–1003）也住在这里。

绑定只订阅 `directory` 段（`xml_curl.conf.xml` 里 `bindings="directory"`），
其他段（configuration/dialplan/…）不走 curl，仍读磁盘。

### 故障降级语义

| 场景 | 行为 |
|------|------|
| control-api 命中分机 | 返回 directory XML，实时生效（**改密码即生效**，无需 reloadxml） |
| control-api 答 `not found`（库里没有 / 非 directory 段） | FS 回退磁盘文件（静态分机 1000–1003 靠这条） |
| control-api 宕机 / 超时（3s） | 该次查询 curl 失败，FS 回退磁盘文件；**已注册的分机不掉线**，靠旧物化文件继续鉴权 |

即：动态是「热路径优化 + 密码不落盘」，磁盘物化是「冷备兜底」。两条都由同一份 DB 事实源驱动，不会漂移。

### 配置项

- 模块启用：`freeswitch/Dockerfile` sed 解注释 `mod_xml_curl`。
- 绑定：`freeswitch/conf/autoload_configs/xml_curl.conf.xml`，`gateway-url` 取
  环境变量 `$${cc_xml_curl_url}`（exec-set 读 `CC_XML_CURL_URL`）。
- URL（含 BasicAuth 凭据）由 compose 注入 `CC_XML_CURL_URL`，两种网络形态：
  - **bridge**（默认）：`http://fs:<CC_API_TOKEN>@control-api:8090/v1/fs-xml`
  - **hostnet**（`freeswitch-host`）：`http://fs:<CC_API_TOKEN>@127.0.0.1:<CC_API_HOST_PORT>/v1/fs-xml`
- 二者默认值已在 compose 里给到位（避免空值）。

> ⚠️ **空值风险待 E2E 验证**：若 `CC_XML_CURL_URL` 为空，`gateway-url` 变量为空串时
> mod_xml_curl binding 的行为未实测（可能 binding 失效或每次查询报错）。因此 compose
> 默认值直接给到位、不留空；改动这块请先在测试机跑一遍注册 + 呼叫再上线。

### 为什么不做 xml_rpc / LDAP（留档）

- **mod_xml_rpc**：是「FS 对外暴露 RPC 让别人控制 FS」的方向，与本需求（FS 主动拉
  目录）相反；且要额外开 HTTP 管理端口、鉴权面更大，不划算。
- **LDAP（mod_ldap / directory LDAP backend）**：需要额外跑一套 LDAP 服务、维护 schema
  与同步，运维成本远高于「一张 Postgres 表 + 一个 HTTP handler」；本项目分机规模小、
  事实源已在 Postgres，xml_curl 直连库最短路径。
- 结论：xml_curl 单轨即满足「密码不落盘 + 实时生效 + 文件兜底」，其余方案不引入。

---

## 🔗 对接 Go Admin Kit

见 [`docs/integration-go-admin-kit.md`](docs/integration-go-admin-kit.md)。

后续在 Admin Kit 增加 `cc-adapter` 接收：

- `call.ringing` / `call.answered` / `call.hangup` / `recording.ready` / `agent.status`

---

## 📊 监控指标

`control-api` 内置每 **10s** 一轮的 ESL 采集器，把 FreeSWITCH 运行态导出为
Prometheus 指标，`GET /metrics` 暴露（**在鉴权组外**——Prometheus 抓取不带
`X-CC-Token`；端点仅在内网 compose/宿主网络可达，不对公网开放）。

一轮采集整体失败时 `fscc_esl_up=0`，其余业务指标保留上一轮值（避免抖动）。

| 指标 | 类型 | 标签 | 说明 |
|------|------|------|------|
| `fscc_channels_total` | gauge | — | 当前活动通道数（`show channels count`） |
| `fscc_calls_total` | gauge | — | 当前活动呼叫数（`show calls count`） |
| `fscc_queue_members` | gauge | `queue` | 各队列当前排队人数 |
| `fscc_queue_longest_wait_seconds` | gauge | `queue` | 各队列当前最长等待时长（秒） |
| `fscc_queue_calls_answered_total` | gauge | `queue` | 各队列累计接通数 |
| `fscc_queue_calls_abandoned_total` | gauge | `queue` | 各队列累计放弃数 |
| `fscc_agents` | gauge | `status` | 按状态统计的坐席数（Available / On Break / Logged Out…） |
| `fscc_gateway_up` | gauge | `gateway` | 网关注册：REGED/NOREG=1（进程存活），其余=0 |
| `fscc_esl_up` | gauge | — | 最近一轮 ESL 采集是否成功（1/0） |

**接入监控栈**：`platform/deploy/prometheus/prometheus.yml` 已加 `fscc-control-api`
抓取 job（target 指向部署环境的 `control-api:8090`）；Grafana 看板
「呼叫中心」随 `platform/deploy/grafana/provisioning/dashboards/` 自动加载
（通道并发、各队列排队深度/最长等待 60s 变色、坐席状态分布、网关注册表、
接通/放弃曲线、ESL 状态灯）。

**话单磁盘兜底**：`mod_json_cdr` 走 HTTP 投递给 cc-service 之外，`mod_cdr_csv`
把每通挂机话单再写一份本地 CSV 到 `/var/lib/freeswitch/cdr-csv`
（docker volume `fscc_cdr_csv`，只记 a-leg，`rotate-on-hup` 支持轮转）。
投递失败或中台离线时可从磁盘补账。

**刻意不做**（当前一人维护、内网单机，够用即止）：

- `mod_snmp`：SNMP 采集器已被 Prometheus 拉模式覆盖，再引一套 MIB/trap 徒增运维面。
- `mod_event_multicast`：多机事件组播用于 FS 集群互通，当前单实例无第二节点。
- AMQP（`mod_amqp` / `mod_rabbitmq`）：话单/事件出口已有 json_cdr + CSV + ESL 订阅三路，
  再挂消息总线属过度设计；有跨系统异步需求时再引。
- Graylog2（`mod_graylog2`）：日志集中化用宿主 `docker logs` + 后续统一日志方案即可，
  不在 FS 侧单独接 GELF。

---

## 🛠️ 部署与调试

完整步骤（开发 / 生产 / `fs_cli` / 软电话 / 抓包）见：

👉 **[`docs/deploy-and-debug.md`](docs/deploy-and-debug.md)**

一句话：

| 场景 | 怎么做 |
|------|--------|
| 本机跑起来 | `make fs-up` 或 `cd freeswitch-cc && docker compose up -d --build` |
| 看是否正常 | `curl :8090/health` + Token 调 `/ready`、`/v1/esl/status` |
| 打电话调试 | 软电话注册 1000/1001，互拨；`fs_cli` → `show registrations` |
| 中台控制 | 中台调 control-api（Token）；事件以后 Webhook 回中台 |
| 生产 | FS 建议独立机器；WebPhone 用可信 WSS；ESL/8090/DB **不对公网** |

---

## 🗺️ 里程碑

| 编号 | 内容 | 状态 |
|------|------|:----:|
| FS-M1 | Compose + 内线 + 录音目录 + control-api + 对接文档 | ✅ 骨架 |
| FS-M2 | 真实 CHANNEL 事件 → Webhook → Kit 话单（常驻 ESL 订阅，A-leg 去重，断线重连） | ✅ |
| FS-M3 | callcenter 坐席签入与队列状态 | ⬜ |
| FS-M4 | 来电弹屏字段 + 录音 URL | ⬜ |
| FS-M5 | 生产 HA / 中继文档 | ⬜ |

---

## ⚠️ 安全提示

- 默认分机密码 **仅供开发**，生产务必修改。  
- ESL 不要对公网暴露；`8021` 映射仅限本机调试。  
- `CC_API_TOKEN` / Webhook Secret 与 Admin Kit 共用时使用高强度随机串。  
- 镜像自动生成的 `CN=FreeSWITCH` 自签名证书只供开发；生产必须通过
  `docker-compose.wss.yml` 挂载与 WSS 域名匹配的受信证书。

---

## License

MIT（与 Go Admin Kit 对齐，可按组织调整）。
