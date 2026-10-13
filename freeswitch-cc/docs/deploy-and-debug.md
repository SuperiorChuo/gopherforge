# FreeSWITCH 部署与调试指南

面向 **一人维护**、当前 **FS-M1 骨架**。  
媒体在 `freeswitch-cc/`，中台在 `microservices/`；**控制关系**是中台 → control-api → ESL → FreeSWITCH。

---

## 1. 部署长什么样

### 1.1 开发机（本机 Docker）

```text
你的 Mac / Linux
├── Docker
│   ├── fscc-freeswitch     SIP 5060 / ESL 8021 / RTP 16384-16484
│   ├── fscc-control-api    HTTP 8090
│   └── fscc-postgres       5434（仅 control-api 话单）
├── 软电话 A 注册 1000
└── 软电话 B 注册 1001
```

与 Admin Kit **可以同机并行**（端口已错开：Kit 用 5432/8000，FS 用 5434/5060/8090）。

### 1.2 生产（推荐拆机）

```text
机器 A：Go Admin Kit（网关 + 微服务 + 前端）
机器 B：freeswitch-cc（FS + control-api + 可选独立 PG）
        ↑ 仅内网开放 8090(control-api) 给机器 A
        ↑ SIP/RTP 对坐席网/中继网开放，ESL 不对公网
```

原则：

| 端口 | 是否对公网 |
|------|------------|
| SIP 5060、RTP | 按业务需要（坐席网/运营商） |
| SIP WebSocket 5066 | 仅开发或受信内网；生产浏览器不用明文 WS |
| SIP WebSocket Secure 7443 | 按 WebPhone 用户范围开放，必须使用可信证书 |
| ESL 8021 | **否**，仅 control-api 所在内网 |
| control-api 8090 | **否**，仅中台内网 + Token |
| Postgres 5434 | **否** |

---

## 2. 开发环境怎么部署

```bash
cd /path/to/go-admin-kit
cd freeswitch-cc
cp -n .env.example .env
docker compose up -d --build
docker compose ps
docker compose logs -f freeswitch
```

或仓库根：

```bash
make fs-up
```

### 健康检查

```bash
# control-api 进程
curl -s http://127.0.0.1:8090/health

# ESL + DB 是否通（需 Token）
curl -s -H "X-CC-Token: dev-cc-token-change-me" \
  http://127.0.0.1:8090/ready

# 分机清单
curl -s -H "X-CC-Token: dev-cc-token-change-me" \
  http://127.0.0.1:8090/v1/extensions | jq .

# FS status（经 ESL）
curl -s -H "X-CC-Token: dev-cc-token-change-me" \
  http://127.0.0.1:8090/v1/esl/status | jq -r .output
```

`.env` 里 `CC_API_TOKEN` 改过之后，上面 Token 一起改。

### 常见启动问题

| 现象 | 处理 |
|------|------|
| 镜像 pull/build 失败 | 检查网络；`safarov/freeswitch` 可换源或私有镜像 |
| 5060 端口占用 | 改 compose 映射或停掉本机别的 SIP |
| macOS 上 RTP/单向通话 | 常见 NAT 问题；先本机双软电话同网测试；生产用 host 网或正确外网 IP |
| control-api ready 里 esl_ok=false | FS 未就绪，等 20～60s；`docker logs fscc-freeswitch` |
| 配置改了不生效 | `docker compose up -d --build freeswitch` 重建；或进容器 `fs_cli -x "reloadxml"` |

---

## 3. 怎么调试（从易到难）

### 3.1 看容器与日志

```bash
cd freeswitch-cc
docker compose ps
docker compose logs --tail=100 freeswitch
docker compose logs --tail=100 control-api

# 进 FS 容器（名称以 ps 为准）
docker exec -it fscc-freeswitch bash
# 或
docker exec -it fscc-freeswitch fs_cli
```

### 3.2 用 `fs_cli`（最重要）

在容器内：

```text
fs_cli
```

常用命令：

| 命令 | 作用 |
|------|------|
| `status` | 总览 |
| `sofia status` | SIP 协议栈 |
| `sofia status profile internal` | 内线 profile |
| `show registrations` | 谁注册上来了 |
| `show channels` | 当前通话通道 |
| `show calls` | 呼叫 |
| `reloadxml` | 重载 XML 配置 |
| `fsctl shutdown` | 关闭（慎用） |

**注册不上时**：先 `show registrations` 是否为空，再查软电话域名/密码/传输是否 UDP、服务器是否填对 IP。

### 3.3 软电话联调步骤（推荐顺序）

1. 起 compose，确认 `8090/health` OK。  
2. 装 **Linphone** 或 **Zoiper**。  
3. 账号 1：用户 `1000` 密码 `mNDhmMVpzZ6p`，服务器 `127.0.0.1`（或局域网 IP），传输 UDP。  
4. 账号 2：`1001` / `pdE4QKmAdQAJ` 同服务器。  
5. `fs_cli` → `show registrations` 应看到 1000、1001。  
6. 1000 拨 `1001`，应振铃/接通。  
7. 1000 拨 `5000`，走演示队列（依次找 1000–1003）。  
8. 录音落在 named volume `fscc_recordings`（`docker exec fscc-freeswitch ls /var/lib/freeswitch/recordings`，或后台「呼叫控制台」录音卡片直接播放；不再用 `./recordings` 绑定——会被 dev-sync --delete 清掉）。

### 3.4 用 control-api 调试（不进 fs_cli 时）

```bash
TOKEN=dev-cc-token-change-me

# ESL 原始 status
curl -s -H "X-CC-Token: $TOKEN" \
  http://127.0.0.1:8090/v1/esl/status | jq -r .output

# 白名单 ESL 命令
curl -s -H "X-CC-Token: $TOKEN" -H "Content-Type: application/json" \
  -d '{"command":"show registrations"}' \
  http://127.0.0.1:8090/v1/esl/api | jq -r .output

# 模拟呼叫事件入库 +（若配置了）Webhook
curl -s -H "X-CC-Token: $TOKEN" -H "Content-Type: application/json" \
  -d '{
    "event":"call.hangup",
    "payload":{
      "call_id":"demo-001",
      "direction":"internal",
      "caller":"1000",
      "callee":"1001",
      "agent_ext":"1001",
      "duration_sec":12
    }
  }' \
  http://127.0.0.1:8090/v1/events/ingest | jq .

# 查本地话单
curl -s -H "X-CC-Token: $TOKEN" http://127.0.0.1:8090/v1/cdr | jq .
```

### 3.5 抓包（SIP 疑难）

```bash
# 本机（需权限）
sudo tcpdump -i any port 5060 -nn

# 或容器内
docker exec -it fscc-freeswitch sh -c "tcpdump -i any port 5060 -nn"
```

看 REGISTER 是否 200、INVITE 是否到对端。

### 3.6 改拨号计划怎么调

1. 改 `freeswitch/conf/dialplan/...` 或 `directory/...`  
2. 重建：`docker compose up -d --build freeswitch`  
3. 或进容器 `fs_cli -x "reloadxml"`（仅 XML 热更时）  
4. 再打一通电话验证  

文档：`docs/dialplan.md`。

### 3.7 和中台联调（以后）

```text
中台 cc-adapter ──HTTP──► control-api:8090
control-api ──Webhook──► 中台 /api/v1/cc/webhook
```

现在可先：

```bash
# .env
CC_WEBHOOK_URL=http://host.docker.internal:8080/你的接收地址
curl -H "X-CC-Token: $TOKEN" -X POST http://127.0.0.1:8090/v1/webhooks/test
```

契约见 `docs/integration-go-admin-kit.md`。

---

## 4. 生产部署怎么做（简版）

### 4.1 单机 all-in-one（小流量）

同一台机：

```bash
# 中台
cd microservices && docker compose up -d

# 媒体
cd ../freeswitch-cc && docker compose up -d
```

防火墙只暴露：中台 443、SIP/RTP（按需）。**不要**暴露 8021、5434、8090 到公网。

### 4.2 双机（推荐）

**机器 B（呼叫机）**

```bash
cd freeswitch-cc
# 生产改分机密码、ESL 密码、CC_API_TOKEN
docker compose up -d --build
```

**机器 A（中台）**

- 配置 `CC_FS_API_BASE=http://机器B内网IP:8090`
- `CC_FS_TOKEN=与 B 相同`
- 以后 `cc-adapter` 收 Webhook：B 上 `CC_WEBHOOK_URL=http://机器A内网/api/v1/cc/webhook`

### 4.3 WebPhone 生产 WSS

`/api/v1/cc/ws` 是呼叫事件通道；WebPhone 注册、呼叫信令走的是 FreeSWITCH
`sofia internal` 的 SIP over WebSocket，两者不是同一个端点。HTTPS 管理台必须配置
独立的 `wss://` 地址，WSS 握手成功也不代表 RTP 双向音频已经可用。

1. 为呼叫机准备 DNS 名称（例如 `pbx.example.com`），让 DNS 指向呼叫机公网入口，
   并把 `7443/tcp` 转发到 FreeSWITCH；必须从公司网络之外实测，路由器内网回环结果
   不能代替公网验证。随后签发浏览器信任且 SAN 包含该名称的证书。镜像自动生成的
   `CN=FreeSWITCH` 自签名证书不能用于生产。
2. 在仓库外生成 FreeSWITCH 要求的合并 PEM，顺序必须是私钥、站点证书、其余证书链：

```bash
install -d -m 700 /etc/go-admin-kit/freeswitch
umask 077
cat /etc/letsencrypt/live/pbx.example.com/privkey.pem \
    /etc/letsencrypt/live/pbx.example.com/fullchain.pem \
  > /etc/go-admin-kit/freeswitch/wss.pem
chmod 600 /etc/go-admin-kit/freeswitch/wss.pem
```

3. 启动媒体面时叠加生产覆盖文件。host 网络是 Linux 上 SIP/RTP 的推荐形态：

```bash
export FS_WSS_PEM=/etc/go-admin-kit/freeswitch/wss.pem
docker compose -f docker-compose.yml -f docker-compose.wss.yml \
  --profile hostnet up -d --build postgres control-api freeswitch-host
docker exec fscc-freeswitch-host fs_cli -x "sofia status profile internal"
```

`sofia status profile internal` 必须同时显示 `WS-BIND-URL :5066` 和
`WSS-BIND-URL :7443`。生产防火墙应开放 `7443/tcp`，并按需限制 `5066/tcp`。

4. 在中台 `microservices/.env` 设置运行时变量：

```dotenv
CC_SIP_WSS_URL=wss://pbx.example.com:7443
```

Compose/Swarm 会把该值传给前端容器，并在容器启动时生成不可缓存的
`runtime-config.js`；因此证书续期或地址切换不需要重编译前端。变量不是 `wss://`
或含异常字符时，前端容器会直接启动失败。

5. 用严格验收器检查系统信任、域名、TLS 和 SIP WebSocket 握手：

```bash
cd microservices
npm run smoke:cc-wss -- wss://pbx.example.com:7443
```

   一键就绪自检（在 FreeSWITCH 宿主机上跑，只读，输出分项 PASS/WARN/FAIL 与结论）：

```bash
bash freeswitch-cc/scripts/check-wss-prod.sh wss://pbx.example.com:7443
```

   不传端点则从 `microservices/.env` 的 `CC_SIP_WSS_URL` 读取；该脚本把上面的媒体面监听、
   证书身份/有效期/SAN、前端运行时注入、严格验收器全部收敛成一条命令，证书/NAT 到位后
   跑一遍即可判断生产 WSS 还差哪一步。

私有 CA 场景可用 `NODE_EXTRA_CA_CERTS=/path/to/company-ca.pem` 检查，但所有实际坐席
浏览器和操作系统也必须安装同一 CA；只给命令行加 CA 不算完成。证书续期后重新生成
`wss.pem`，滚动重启 `freeswitch-host`，再重跑严格验收。

最终业务验收还要使用两部分机完成：注册、呼入、外呼、双向音频、DTMF、保持/恢复、
盲转和协商转。若信令通过但无声，继续检查 `16384-16484/udp`、外网 RTP IP 和 NAT。

### 4.4 配置检查清单（上线前）

- [ ] 分机密码不是 `1234`  
- [ ] ESL 密码不是 `ClueCon`，且仅内网  
- [ ] `CC_API_TOKEN` / Webhook Secret 高强度  
- [ ] 录音盘容量与备份策略  
- [ ] 中继/外线账号不进 Git  
- [ ] 时区 `TZ=Asia/Shanghai`  
- [ ] 日志与磁盘监控  
- [ ] `7443/tcp` 使用域名匹配的可信证书，严格 WSS 验收通过
- [ ] 从公司网络之外确认 WSS 域名的 `7443/tcp` NAT/防火墙可达
- [ ] WebPhone 注册、呼入/外呼、双向音频、DTMF、转接实测通过

### 4.5 升级 FreeSWITCH / 拨号计划

```bash
cd freeswitch-cc
# 改 conf 或 Dockerfile 基础镜像版本
docker compose build freeswitch
docker compose up -d freeswitch
# 观察注册是否掉线，再抽测互拨
```

control-api 与 FS **可分开升级**，这正是拆目录的意义。

---

## 5. 调试心智模型（出问题先问哪）

```text
注册失败？     → 软电话配置 / 5060 / show registrations
能注册不能打？ → dialplan / show channels / 本机防火墙 RTP
control-api 挂？→ logs control-api / DB 5434
中台控不了？   → Token、8090 网络、ready 里 esl_ok
没有录音？     → recordings 卷权限、execute_on_answer 是否执行
```

---

## 6. 和「中台控制 FS」对应的调试

| 你想验证 | 怎么做 |
|----------|--------|
| 中台「能连上媒体」 | `GET /ready` 中 `esl_ok=true` |
| 中台「能查状态」 | `GET /v1/esl/status` |
| 中台「能收事件」 | `POST /v1/webhooks/test` + 看中台日志 |
| 中台「写话单」 | `POST /v1/events/ingest` + `GET /v1/cdr` |

目前 **FS-M1** 已具备控制面骨架；**真实每通电话自动推 Webhook** 属于 FS-M2（订 ESL 事件再转发）。

---

## 7. 常用一键命令摘要

```bash
# 启动 / 停止
make fs-up
make fs-down

# 日志
cd freeswitch-cc && docker compose logs -f freeswitch control-api

# 控制面
export T=dev-cc-token-change-me
curl -s -H "X-CC-Token: $T" http://127.0.0.1:8090/ready
curl -s -H "X-CC-Token: $T" http://127.0.0.1:8090/v1/extensions

# FS 控制台
docker exec -it fscc-freeswitch fs_cli
```
