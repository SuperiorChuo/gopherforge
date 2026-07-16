# 远程热更新开发（内网 `192.168.220.109`）

> 参考 **kingYM / go-scaffold**：本机写代码 → rsync → 服务器运行。  
> **当前策略：只部署 monorepo 微服务 Docker，不部署单体。** 旧 `/www/go-admin-kit/stack` 由 `remote-ms-deploy` 替换。

## 架构

```
┌─ Mac ──────────────────┐     rsync      ┌─ 192.168.220.109 ──────────────────┐
│ 编辑 monorepo          │ ─────────────► │ /www/go-admin-kit/src              │
│ scripts/dev-sync.sh    │                │  microservices/ docker compose     │
│                        │                │   gateway :18100  frontend :13100  │
│                        │                │  gak-ms-web-dev vite HMR :13200    │
│ 浏览器                 │ ◄───────────── │  （postgres volume 沿用，数据保留） │
└────────────────────────┘                └───────────────────────────────────┘
```

| 路径 | 说明 |
|------|------|
| `/www/go-admin-kit/src` | monorepo 源码（dev-sync 推送） |
| `/www/go-admin-kit/stack` | 旧 compose 目录（已停用；`.env` 仍被部署脚本读取密钥） |

## 端口

| 服务 | 端口 | 说明 |
|------|------|------|
| Traefik 网关 | **18100** | API + 路由（主入口） |
| 前端静态（compose） | **13100** | `docker compose build frontend` 产物 |
| 前端 Vite HMR | **13200** | 改 React 热更新，代理到 18100 |
| Postgres / Redis | 15434 / 16380 | 与旧栈相同，**数据卷保留** |
| MinIO | 19000 / 19001 | 避开 sku 占用的 9000 |
| IM | 18088 | im-service 直连（通常走网关） |

## 首次 / 替换旧栈

```bash
# 停旧 stack、启 monorepo microservices（保留 DB volume）
./scripts/remote-ms-deploy.sh

# 前端 Vite 单元（可选，改 React 用）
scp deploy/remote/gak-ms-web-dev.service root@192.168.220.109:/etc/systemd/system/
ssh root@192.168.220.109 "cd /www/go-admin-kit/src/microservices/web && pnpm i && systemctl daemon-reload && systemctl enable --now gak-ms-web-dev"
```

## 本机日常

```bash
# 代码热推
./scripts/dev-sync.sh

# 后端改完需要重建镜像时：
./scripts/remote-ms-deploy.sh
# 或只重建某个服务：
ssh root@192.168.220.109 "cd /www/go-admin-kit/src/microservices && docker compose up -d --build identity-service"

# 前端改完必须重建静态 13100（用户默认验收入口）：
ssh root@192.168.220.109 "cd /www/go-admin-kit/src/microservices && export COMPOSE_PROJECT_NAME=go-admin-kit && docker compose up -d --build frontend"
```

免密：`~/.ssh/id_ed25519`。**不要把 root 密码写进仓库。**

> **协作约定（强制）**：改完会影响远端效果的代码后，AI / 开发者须按 `AGENTS.md`「远程热更新部署」一节推送并重建，不要只改本地。

## 访问

| URL | 用途 |
|-----|------|
| http://192.168.220.109:18100 | 网关（API + 静态前端路由） |
| http://192.168.220.109:13100 | compose 静态前端 |
| http://192.168.220.109:13200 | Vite HMR（开发改 UI） |

## 运维

```bash
ssh root@192.168.220.109 "cd /www/go-admin-kit/src/microservices && docker compose ps"
ssh root@192.168.220.109 "cd /www/go-admin-kit/src/microservices && docker compose logs -f auth-service"
ssh root@192.168.220.109 "journalctl -u gak-ms-web-dev -f"
```

## 踩坑

| 现象 | 处理 |
|------|------|
| 9000 端口冲突 | `.env` 里 MinIO 用 19000/19001 |
| 数据丢了 | 部署脚本 `compose down` **不加 -v**；volume 名 `go-admin-kit_go_admin_kit_postgres_data` |
| 权限/密钥 | 从 `stack/.env` 复制，勿提交 git |
| 前端改了没变 | 用 13200 HMR；或重建 `frontend` 镜像 |
| Desktop launchd 读不了 | 用终端跑 `dev-sync` 或 zshrc |
