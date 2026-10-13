# 部署完整产品

v0.8.0 候选包括 16 个 API 服务、AI worker 和 React 前端；媒体和 Hermes 客户端单独启动。当前正式版及可用镜像以 [Releases](https://github.com/SuperiorChuo/gopherforge/releases) 为准，候选分支不代表已经发布。

## 准备

- Linux、Docker Engine **24+**、Compose v2；建议至少 8C16G，实际容量需压测。
- TLS 域名、强随机密钥、数据库和上传的备份/恢复方案。
- 按 `.env.example` 配置 `APP_ENV=production`、JWT、PG、Redis 和内部令牌；禁止示例密码。
- 服务端口保持 loopback 绑定，公网请求通过 HTTPS 反代到 Traefik。
- AI、支付、公众号、短信和电话供应商需自行配置；源码存在不代表外部链路可用。

## 源码构建

```bash
git clone https://github.com/SuperiorChuo/gopherforge.git
cd gopherforge
cp microservices/.env.example microservices/.env
# 先编辑安全配置
make compose-up
```

`make compose-up` 创建共享网络、先起独立 infra，再以 `compose-fallback` profile 启动应用。网关默认 `http://localhost:8000`；正式入口需另配 TLS。初始账号 `admin` / `admin123`，立即改密。

## 正式版镜像

发布工作流按 Compose 动态构造镜像矩阵。完整主应用为 17 个唯一镜像：16 API + frontend；worker 和 migrate 复用 API 镜像。媒体与客户端另行交付。历史上自 **v0.3.0** 起提供双架构，但每次仍须核对实际镜像及公开 SHA。

以下仅在 v0.8.0 镜像正式发布后使用：

```bash
cd microservices
export IMAGE_PREFIX=ghcr.io/superiorchuo/gopherforge/go-admin-kit
export IMAGE_TAG=v0.8.0
docker compose --profile compose-fallback pull
docker compose --profile compose-fallback up -d --no-build
```

## 可观测与独立组件

```bash
# 应用栈
docker compose --profile compose-fallback --profile monitoring up -d
docker compose --profile compose-fallback --profile tracing up -d
# 数据栈的可选存储
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml --profile storage up -d
```

[完整部署指南](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/deployment.md)包含 Nginx/HTTPS 示例、独立媒体和客户端入口。管理面板不得裸露公网。

## 升级与恢复

- 固定版本和完整公开 SHA，先备份，在隔离副本演练。
- 公开历史 SQL 保持原路径、原字节；完整产品仅追加 forward-only 迁移，禁止生产 `down/reset`。
- 应用回退不等于 schema 回退。需要恢复时使用演练过的匹配数据库备份和应用版本，不能只切旧镜像。
- 健康检查不是完整验收：还需 API、worker、UI、实时监控与所需供应商链路。
- 同机隔离环境必须独立网络、卷、容器名、端口与凭据，不可复用生产数据。

参见[升级指南](/reference/upgrade)、[可观测](/modules/observability)和[发行验证](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/release-validation.md)。
