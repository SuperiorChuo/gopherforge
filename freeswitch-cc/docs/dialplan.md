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

### Linux（默认）

```bash
docker compose up -d --build
```

`freeswitch` 使用 `network_mode: host`。

### macOS / 不便 host 网络

```bash
docker compose stop freeswitch 2>/dev/null
docker compose --profile bridge up -d --build freeswitch-bridge control-api postgres
```

并将 `.env` 中：

```env
CC_ESL_HOST=freeswitch-bridge
```

（control-api 与 bridge FS 在同一 compose 网络。）

## ESL

- 端口：`8021`
- 默认密码：`ClueCon`（务必改）
- 配置：`freeswitch/conf/autoload_configs/event_socket.conf.xml`
