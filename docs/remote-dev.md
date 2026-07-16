# 远程热更新开发（内网 `192.168.220.109`）

> 参考本机 **kingYM**（`docs/remote-dev-guide.md`）与 **go-scaffold** 的实践：  
> **本机只写代码 → rsync 推送 → 服务器 air / vite 热重载 → 浏览器看效果**。

## 架构

```
┌─ Mac ──────────────────┐     rsync      ┌─ 192.168.220.109 ─────────────┐
│ 编辑 monorepo          │ ─────────────► │ /www/go-admin-kit/src         │
│ scripts/dev-sync.sh    │                │  gak-mono-api-dev (air :18201)│
│                        │                │  gak-ms-web-dev   (vite :13200)│
│ 浏览器打开服务器 IP     │ ◄───────────── │  (可选) stack docker 网关:18100│
└────────────────────────┘                └───────────────────────────────┘
```

| 路径 | 说明 |
|------|------|
| `/www/go-admin-kit/src` | monorepo 源码（本脚本同步） |
| `/www/go-admin-kit/stack` | **旧版** docker 全栈（勿被 rsync 覆盖） |

## 端口（避开已占用）

| 服务 | 端口 | 说明 |
|------|------|------|
| 单体 API（air） | **18201** | 热重载后端 |
| 微服务前端（vite） | **13200** | HMR；API 代理到 `18100` 网关 |
| 现有 docker 网关 | 18100 | 旧 stack，可继续给前端代理 |
| 现有 docker 前端静态 | 13100 | 旧构建产物 |

## 本机用法

```bash
# 首次 / 单次全量
./scripts/dev-sync.sh once

# 常驻热推（建议单独开终端）
./scripts/dev-sync.sh
# 日志
tail -f /tmp/go-admin-kit-devsync.log   # 若用 nohup 重定向
```

免密：使用本机 `~/.ssh/id_ed25519`（已可登录 root@192.168.220.109）。  
**不要把 root 密码写进仓库或脚本。**

### 可选：zshrc 自启同步

```bash
# ~/.zshrc
if [ -x "$HOME/Desktop/github/go-admin-kit/scripts/dev-sync.sh" ] \
  && ! pgrep -qf "go-admin-kit/scripts/dev-sync.sh"; then
  (nohup "$HOME/Desktop/github/go-admin-kit/scripts/dev-sync.sh" \
    >/tmp/go-admin-kit-devsync.log 2>&1 &)
fi
```

## 服务器一次性安装

```bash
# 1. 单元文件
scp deploy/remote/gak-mono-api-dev.service root@192.168.220.109:/etc/systemd/system/
scp deploy/remote/gak-ms-web-dev.service root@192.168.220.109:/etc/systemd/system/

# 2. 远端依赖与配置（见 scripts/remote-bootstrap.sh）
./scripts/remote-bootstrap.sh
```

## 日常运维

```bash
ssh root@192.168.220.109 "journalctl -u gak-mono-api-dev -f"
ssh root@192.168.220.109 "journalctl -u gak-ms-web-dev -f"
ssh root@192.168.220.109 "systemctl restart gak-mono-api-dev"
ssh root@192.168.220.109 "cd /www/go-admin-kit/src/microservices/web && pnpm install"
```

浏览器：

- 前端 HMR：`http://192.168.220.109:13200`
- 单体 API：`http://192.168.220.109:18201/api/v1/health`（若配置了 health）
- 旧网关：`http://192.168.220.109:18100`

## 与微服务 Docker 的关系

- **热更主路径（推荐日常）**：单体 air + 微服务 web vite（本页）。  
- **完整微服务栈**：继续用 `/www/go-admin-kit/stack` 的 docker compose，或后续把 monorepo `microservices/docker-compose.yml` 迁到远端另起项目名（注意端口冲突）。  
- IM / 多租户等 **实验特性** 在 monorepo 微服务线；单体热更不含完整 IM。

## 踩坑

| 现象 | 处理 |
|------|------|
| Permission denied | 用 `id_ed25519` 免密；勿只依赖密码 |
| Go module cache not found | systemd 必须设 HOME/GOPATH/GOMODCACHE/GOCACHE |
| vite 外网访问不了 | 必须 `vite --host 0.0.0.0`（`dev:lan`） |
| .env 被覆盖 | rsync 已排除；服务器手工维护 |
| Desktop 下 launchd 读不了目录 | 用 zshrc 启 dev-sync，或把仓库挪出 Desktop |
