# 拨号计划说明（M1）

## 分机

| 号码 | 行为 |
|------|------|
| 1000–1003 | 内线用户，密码 `1234` |
| 5000 | 演示队列：顺序 bridge 到 1000–1003 |

配置文件：

- `freeswitch/conf/directory/default/100x.xml`
- `freeswitch/conf/dialplan/default/01_local_extensions.xml`
- `freeswitch/conf/dialplan/default/02_demo_queue.xml`

## 录音

接通后写入：

```text
/var/lib/freeswitch/recordings/<uuid>.wav
```

Compose 映射到宿主机 `./recordings/`。

## Compose 网络模式

### 默认（macOS / Windows / 一般 Linux）

```bash
docker compose up -d --build
```

`freeswitch` 使用 **bridge + 端口映射**（5060/8021/RTP）。control-api 默认 `CC_ESL_HOST=freeswitch`。

### Linux 需要 host 网络时（SIP/RTP 更省心）

```bash
docker compose --profile hostnet up -d --build freeswitch-host
# control-api 的 CC_ESL_HOST 改为 host.docker.internal 或宿主机可达地址
```

## ESL

- 端口：`8021`
- 默认密码：`ClueCon`（务必改）
- 配置：`freeswitch/conf/autoload_configs/event_socket.conf.xml`
