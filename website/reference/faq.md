---
description: GopherForge 启动、端口、Apple Silicon、迁移、生产配置、监控与 BPM 接入的常见问题。
---

# 常见问题 FAQ

## 启动与运行

### `make compose-up` 之后应用起来了，数据库容器在哪？

栈是**两层**的：有状态服务（PostgreSQL / Redis / NATS / MinIO）在独立的 infra 栈里，compose project 名固定为 `go-admin-kit-infra`。所以：

```bash
docker compose ps                                              # 只看应用栈
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml ps   # 看数据栈
```

`docker compose down` 只会停应用栈，**数据永远不受影响**——这是刻意设计。想彻底重置数据才需要：

```bash
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml down -v   # ⚠️ 删数据卷
```

操作 infra 栈必须显式带 `-p go-admin-kit-infra`，否则 compose 会把另一栈的容器当孤儿清理。

### 端口被占了怎么办？

默认端口：前端 `3000`、网关 `8000`、PostgreSQL `5432`、Redis `6379`。改 `microservices/.env` 里对应的 `FRONTEND_PORT` / `GATEWAY_PORT` / `POSTGRES_PORT` / `REDIS_PORT` 再 up 即可，compose 全部读变量。

### Apple Silicon（M 系列）能跑吗？

能，本地构建（`make compose-up`）全链路支持 arm64。两个注意点：

1. Docker Desktop for Mac 需要在 `.env` 设 `DOCKER_SOCK=/var/run/docker.sock.raw`（默认 sock 形态会拒绝访问）；
2. 官方 ghcr 镜像 **`v0.3.0` 起提供 `linux/arm64`**，Apple Silicon / arm64 云机可直接镜像部署；`v0.2.0` 及更早仅 amd64（老版本请本地构建）。

### migrate 容器退出了，正常吗？

正常。`migrate` 是一次性任务：等 PG 就绪（最多重试 30 次）→ 跑 goose 迁移 → 以 0 退出；业务服务通过 `service_completed_successfully` 等它完成后才启动。它以非零码退出才是问题——看 `docker compose logs migrate`。

### 健康检查端点是哪个？

统一 `GET /api/v1/health/ready`（经网关 `http://localhost:8000/api/v1/health/ready`）；bpm 服务是 `GET /api/v1/bpm/health/ready`。K8s liveness/readiness 探针也用它们。

### NATS 是可选的吗？

不可选。登录日志链路依赖它（auth 发布登录事件 → audit 持久消费落库），去掉 NATS 会丢登录审计。

## 生产环境

### `APP_ENV=production` 下服务拒绝启动，报配置错误？

这是**弱凭据护栏**在工作，不是 bug。生产模式启动期校验（报错会指明哪项不合格）：

| 配置 | 校验条件 |
|------|---------|
| `JWT_SECRET` | 恒校验：≥32 位且非占位符 |
| `POSTGRES_PASSWORD` | 恒校验：非空、非 `123456` 等弱值 |
| `REDIS_PASSWORD` | 恒校验（连 Redis 的服务）：非弱值 |
| 对象存储 access/secret key | 仅 `UPLOAD_STORAGE_TYPE=s3/minio` 时校验（`minioadmin` 会被拦） |
| OAuth `CLIENT_SECRET` | 仅开关开启且三件套齐备时校验 |
| SMTP 密码 | 仅邮件通道开启、host 非空且确实要发 AUTH 时校验 |

功能没启用就不校验——不会因为不用 OAuth 而被 OAuth 校验卡住。

### 默认账号是什么？必须改吗？

`admin / admin123`，仅供本地开发。生产首次登录后立即在「系统管理 → 用户」改密码——上线检查清单里有这条。

### 登录日志里归属地是空的？

没下载 IP 离线库。跑 `scripts/download-ip2region.sh`（下载 `ip2region.xdb` 到 `microservices/data/`，约 11MB）后重启 `system-service` 与 `audit-service`。不下载也能用：登录日志回退在线查询（有 1 小时缓存），在线用户页归属地留空。

### 有 CSRF 防护吗？

**没有实现**，这是文档化的已知边界：前端是纯 SPA + `Authorization: Bearer` 头，天然把 CSRF 面收得很窄。但如果你二开时改成 cookie 承载 token（尤其 `SameSite=None`），必须自行补 CSRF token 或 Origin 校验。

### `/metrics` 怎么访问不到？

刻意的：全服务的 Prometheus `/metrics` 只在容器网络内可达，网关不路由它。要看指标就启 monitoring profile（`docker compose --profile monitoring up -d`），Grafana 里有预置「服务概览」看板；`METRICS_ENABLED=false` 可整体关闭。

## 功能相关

### BPM 的 internal 端点返回 503？

`BPM_INTERNAL_TOKEN` 未配置或还是占位符。这是 fail-closed 设计：宁可业务侧接不进来，也不拿公开的开发占位符当真凭据。配上强随机 token（两侧一致）即恢复。

### 冒烟测试怎么跑？

```bash
cd microservices
npm run test:smoke:unit && npm run test:contract      # 本地即可
API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api   # 需要网关已就绪
```

`smoke:api` 内置真实验证码识别器，会走完「取验证码 → 登录 → 接口冒烟」全链路，不需要人工干预。

### 想要的答案不在这页？

- 部署类问题 → [生产部署](/reference/deployment)
- 升级类问题 → [版本升级](/reference/upgrade)
- 二开类问题 → [二次开发](/guide/extend)
- 都没有 → [GitHub Issues](https://github.com/SuperiorChuo/gopherforge/issues) 提问
