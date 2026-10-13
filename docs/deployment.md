# 生产部署（Linux + Docker Compose + HTTPS）

完整产品包含 16 个 API 服务、AI worker 和前端，媒体与客户端另行部署。建议至少 8C16G 起步，具体容量需按业务、模型和监控负载压测；本指南不是容量保证。需要 Docker Engine 24+、Compose v2、域名及 TLS 证书。Kubernetes 见[迁移指南](deploy-k8s.md)，本地开发见[联调说明](../LOCAL_SETUP.md)。

> 当前候选内容面向 v0.8.0，镜像可用性以 [Releases](https://github.com/SuperiorChuo/gopherforge/releases) 为准。未发布候选只能本地构建，不能假定对应镜像已经存在。

## 配置与安全

```bash
git clone https://github.com/SuperiorChuo/gopherforge.git /opt/gopherforge
cd /opt/gopherforge/microservices
cp .env.example .env
chmod 600 .env
```

上线前至少设置：

- `APP_ENV=production`。
- `JWT_SECRET`：至少 32 位、非占位随机密钥，例如 `openssl rand -base64 48`。
- PostgreSQL、Redis、内部服务令牌、MinIO/Grafana（启用时）使用独立强密码，禁止示例值。
- 保持 `SERVICES_BIND_IP=127.0.0.1`，业务端口不得直接暴露给公网。
- `CORS_ALLOW_ORIGINS=https://admin.example.com` 替换为实际来源；带凭据时禁止 `*`。
- 持久化上传和数据库；需要 S3/MinIO 时配置存储端点、桶和凭据。
- AI、公众号、短信、语音供应商按需配置；未配置链路不视为可用。
- Pay 当前仅有 mock 渠道，微信/支付宝适配器尚未实现，不能仅配置凭据用于真实收款。生产保持 `PAY_DEMO_MODE` 关闭；mock 只用于隔离测试/演示。

生产校验会拒绝部分弱配置，但不能替代密钥管理、网络隔离、备份和权限审计。

## 数据栈与应用栈

```bash
cd /opt/gopherforge/microservices
docker network inspect go-admin-kit-net >/dev/null 2>&1 || \
  docker network create --subnet 172.28.0.0/16 go-admin-kit-net
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml up -d
docker compose --profile compose-fallback up -d --build
docker compose --profile compose-fallback ps
```

一次性 `migrate` job 成功后启动应用。默认名称适用于单套环境；同机验收须另设全部容器名、网络、卷和端口，不能复用开发/生产数据。

正式版镜像已发布后可改用：

```bash
export IMAGE_PREFIX=ghcr.io/superiorchuo/gopherforge/go-admin-kit
export IMAGE_TAG=v0.8.0  # 仅在该版本正式发布并核对候选 SHA 后使用
docker compose --profile compose-fallback pull
docker compose --profile compose-fallback up -d --no-build
```

发布矩阵从 Compose 动态生成：完整主应用有 17 个唯一镜像（16 API + frontend）；worker 复用 AI 镜像，migrate 复用 monitor 镜像。媒体与客户端不在此矩阵。历史版本自 v0.3.0 起支持双架构；每次发行仍须核对实际 `linux/amd64`、`linux/arm64` 镜像与发布 SHA，不能仅凭文档判断。

## HTTPS 入口

Traefik 默认 HTTP 入口在 `8000`，应置于 TLS 反代之后；防火墙只开放实际使用的入口，不开放数据库、服务直连或管理面板。

```nginx
server {
    listen 80;
    server_name admin.example.com;
    return 301 https://$host$request_uri;
}
server {
    listen 443 ssl;
    server_name admin.example.com;
    ssl_certificate /etc/letsencrypt/live/admin.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/admin.example.com/privkey.pem;
    client_max_body_size 50m;
    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
    }
}
```

证书路径和域名需替换为实际配置。访问 `https://admin.example.com`；首次账号 `admin` / `admin123`，立即改密。Traefik dashboard、Prometheus 和 Grafana 只允许可信网络访问。

## 可选组件

```bash
# MinIO 属于 infra 栈
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml --profile storage up -d
# 监控与追踪属于应用栈
docker compose --profile compose-fallback --profile monitoring up -d
docker compose --profile compose-fallback --profile tracing up -d
```

[媒体栈](../freeswitch-cc/README.md)和 [Hermes 客户端](../clients/hermes-manager/README.md)分别部署。网络可达、供应商配置和 SIP/RTP/WebRTC 验收不能由 HTTP 健康检查替代。

## 升级、恢复与验收

1. 固定目标版本及完整公开 commit SHA，核对镜像和构建来源。
2. 备份数据库、上传、配置和密钥；在隔离副本演练升级与恢复。
3. 保留全部公开历史 SQL 原路径、原字节，只追加新迁移。v0.8.0 升级为 forward-only；禁止 `down/reset`。
4. 应用回退不等于数据库回退。不得仅切旧镜像继续使用不兼容新 schema；按演练方案恢复匹配的数据库和应用。
5. 检查全部服务、API、worker、UI、监控抓取及所需外部集成；健康接口通过只是其中一项。

```bash
curl --fail http://127.0.0.1:8000/api/v1/health/ready
BASE_URL=http://127.0.0.1:8000 bash ../scripts/slo-probe.sh
```

详细验证与证据绑定见[发行验证](release-validation.md)和[迁移约定](development/MIGRATIONS.md)。停止应用不要删除 infra 数据卷。
