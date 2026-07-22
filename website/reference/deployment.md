# 生产部署

> 本页与仓库 [`docs/deployment.md`](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/deployment.md) 同源。

面向把 **GopherForge 微服务版** 部署到一台 Linux 服务器的运维/自部署用户。本地开发联调请看 [`LOCAL_SETUP.md`](https://github.com/SuperiorChuo/gopherforge/blob/main/LOCAL_SETUP.md)，本文只讲**生产上线**。

---

## 1. 架构与你要准备的东西

一条请求的路径：

```
公网 ──HTTPS──► Nginx（TLS 终止 / 反代）──HTTP──► Traefik 网关(:8000) ──► 各微服务
                                                          │
                        PostgreSQL · Redis · NATS · MinIO(可选)
```

> **关键事实**：内置的 Traefik 网关只监听 HTTP（`--entrypoints.web.address=:80`），**不做 TLS**。生产必须在它前面放一个反向代理（推荐 Nginx）来终止 HTTPS。这是有意的设计——证书与 HTTPS 归属运维层，不塞进应用栈。

**服务器要求**：
- Linux（Ubuntu 22.04 / Debian 12 / 任意 systemd 发行版）
- Docker Engine 24+ 与 Docker Compose v2（`docker compose`，非旧版 `docker-compose`）
- 建议 4C8G 起（全栈 ~13 个 Go 服务 + PG + Redis + NATS）
- 一个域名 + 该域名的 TLS 证书（Let's Encrypt 即可）

---

## 2. 拉取代码与准备 .env

```bash
git clone <your-repo> /opt/go-admin-kit
cd /opt/go-admin-kit/microservices
cp .env.example .env
chmod 600 .env          # .env 含密钥，收紧权限
```

**必须改的项**（`APP_ENV=production` 会强校验这些，弱值直接拒绝启动）：

| 变量 | 要求 | 说明 |
|------|------|------|
| `APP_ENV` | `production` | 开启严格校验（见下） |
| `JWT_SECRET` | **≥32 位、非占位** | 生成：`openssl rand -base64 48` |
| `POSTGRES_PASSWORD` | 强密码、非默认 | 不能是 `123456`/占位符 |
| `REDIS_PASSWORD` | 强密码、非空 | 生产 Redis 必须设密码 |
| `POSTGRES_USER` | 自定义 | 别用默认 `postgres` |

**强烈建议改的项**：
- `SERVICES_BIND_IP=127.0.0.1`（默认值，保持）——业务服务端口只绑 loopback，杜绝内网伪造 `X-Auth-*` 头绕过鉴权。**不要**改成 `0.0.0.0`。
- `CORS_ALLOW_ORIGINS`——改成你的真实域名（如 `https://admin.example.com`），删掉 localhost 项。`CORS_ALLOW_CREDENTIALS=true` 时**不允许**用 `*`。
- `GRAFANA_ADMIN_PASSWORD` / `MINIO_ROOT_PASSWORD`——若启用对应可选栈，改掉默认。
- 对象存储：默认 `UPLOAD_STORAGE_TYPE=local`（文件存容器卷）。生产多副本或要持久化建议 `minio` 或外部 `s3`，并填强 access/secret key（production 校验会拒弱值）。

> **`APP_ENV=production` 严格校验**（`monitor` 启动时执行，任一不过则**整个迁移/启动失败**）：JWT secret ≥32 位非占位、DB 密码非弱、Redis 密码非弱；storage 选 s3/minio 时其 endpoint/bucket/key 必须合法非弱。这是防止"带着 dev 默认值上线"的护栏——报错信息会明确指出哪项不合格。

---

## 3. 启动核心栈

核心服务（无 profile，默认启动）：postgres、redis、nats、migrate（一次性迁移）、9 个业务服务、gateway、frontend。

```bash
cd /opt/go-admin-kit/microservices
docker compose up -d --build
# 等全部 healthy（migrate 会先跑 goose 迁移再退出，业务服务 depends_on 它完成）
docker compose ps
```

健康检查（网关内部）：
```bash
curl -s http://127.0.0.1:8000/api/v1/health/ready   # 期望 {"code":200,...,"status":"ready"}
```

**首次登录默认账号**：`admin` / `admin123`——**登录后立即在「系统管理→用户」改密码**。

### 可选栈（按需）
- **对象存储 MinIO**：`docker compose --profile storage up -d`
- **可观测（Prometheus/Grafana/OTel/Jaeger）**：`docker compose --profile monitoring up -d`（默认不启，见 [ops-gaps]）

### 可选：IP 归属地离线库（登录日志 / 在线用户）
```bash
/opt/go-admin-kit/scripts/download-ip2region.sh   # 下载 ip2region.xdb（约 11MB，不进 git）到 microservices/data/
docker compose restart system-service audit-service
```
文件缺失时服务优雅降级：登录日志回退在线查询、在线用户归属地留空，不影响启动。

---

## 4. Nginx 反向代理 + HTTPS

用 Nginx 终止 TLS，反代到网关的 `${GATEWAY_PORT:-8000}`。示例 `/etc/nginx/conf.d/go-admin-kit.conf`：

```nginx
server {
    listen 80;
    server_name admin.example.com;
    return 301 https://$host$request_uri;   # 强制 HTTPS
}

server {
    listen 443 ssl http2;
    server_name admin.example.com;

    ssl_certificate     /etc/letsencrypt/live/admin.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/admin.example.com/privkey.pem;

    client_max_body_size 50m;   # 文件/素材上传

    location / {
        proxy_pass http://127.0.0.1:8000;   # → Traefik 网关（前端 SPA + /api 都在网关后）
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket（IM 消息 / 系统通知 / 呼叫监控都用）
        proxy_http_version 1.1;
        proxy_set_header Upgrade    $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
    }
}
```

证书用 certbot 签发：`certbot --nginx -d admin.example.com`。

**改完记得同步 `.env` 的 `CORS_ALLOW_ORIGINS=https://admin.example.com` 并重启相关服务**，否则浏览器 POST/预检会 403。

> 网关的 Traefik dashboard 绑在 `127.0.0.1:8090`（仅本机）。不需要就删掉 compose 里 gateway 的 `--api.insecure=true` 和 `127.0.0.1:8090:8080` 端口映射。

---

## 5. 升级 / 回滚

**升级**：
```bash
cd /opt/go-admin-kit/microservices
git pull
docker compose up -d --build           # 迁移由 migrate job 自动跑；只重建有变化的镜像
```

**回滚**（当前 compose 无镜像版本管理，靠 tag 手动留一版）：
```bash
# 升级前先给要动的服务打 prev tag，坏了可回
docker tag go-admin-kit-system-service:latest go-admin-kit-system-service:prev
# 回滚：改 compose image 指向 :prev 或 docker tag 回去后 up -d
```
> 数据库迁移用 goose，**只前进不自动回退**。回滚代码前先确认新迁移是否兼容旧代码；破坏性迁移要先备份再上。

---

## 6. 备份（务必配，当前栈不自带）

数据全在 `go_admin_kit_postgres_data` 卷。**上线第一天就配每日备份**：

```bash
# /etc/cron.daily/go-admin-kit-pgdump（chmod +x）
#!/bin/bash
set -e
OUT=/var/backups/gak/pg-$(date +%F-%H%M).sql.gz
mkdir -p /var/backups/gak
docker exec go-admin-kit-postgres pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" | gzip > "$OUT"
ls -t /var/backups/gak/pg-*.sql.gz | tail -n +8 | xargs -r rm   # 留最近 7 份
```
上传文件（local 存储时）在 `go_admin_kit_uploads`/`im_uploads` 卷，一并纳入备份；用 MinIO/S3 时走对象存储自身的备份策略。

---

## 7. 日志与磁盘

Docker 默认 json-file 日志不轮转，长期会撑满磁盘。配 `/etc/docker/daemon.json`：
```json
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "50m", "max-file": "5" }
}
```
改后 `systemctl restart docker`（会重启容器，择时操作）。

---

## 8. 上线检查清单

- [ ] `.env`：`APP_ENV=production`、JWT/DB/Redis 强密钥、`chmod 600`
- [ ] `SERVICES_BIND_IP` 保持 loopback（未设或 127.0.0.1）
- [ ] `CORS_ALLOW_ORIGINS` 改成真实 HTTPS 域名，无 `*`
- [ ] Nginx HTTPS 反代到 `:8000`，WebSocket upgrade 头齐全
- [ ] `docker compose ps` 全 healthy；`/api/v1/health/ready` 返回 200
- [ ] admin 默认密码已改
- [ ] PG 每日备份 cron 已配并验证能跑出文件
- [ ] Docker 日志轮转已配
- [ ] （可选）monitoring/storage profile 按需启用并改默认密码

---

## 相关文档
- 数据库迁移：[`development/MIGRATIONS.md`](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/development/MIGRATIONS.md)
- 安全说明：[`SECURITY.md`](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/SECURITY.md)
