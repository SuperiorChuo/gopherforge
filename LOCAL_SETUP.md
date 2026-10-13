# 本地联调

本仓为完整微服务产品；不包含单体目录。客户端与媒体栈的独立启动方式见[产品范围](docs/PRODUCT_LINES.md)。

## 环境

- Go 1.27+；Node.js 22.12+（推荐 24）、npm。
- Linux Docker Engine 与 Docker Compose v2。开发机不具备 Docker 时，可使用独立 Linux 测试主机；不要复用已有开发/生产数据库做发行验收。

## 启动

```bash
cp microservices/.env.example microservices/.env
# 检查密码、密钥、端口及业务配置后：
make compose-up
```

该目标创建共享网络并启动独立 infra 栈，再使用 `compose-fallback` profile 启动应用。访问 `http://localhost:8000`，初始账号 `admin` / `admin123`，立即改密。

手动方式：

```bash
cd microservices
docker network inspect go-admin-kit-net >/dev/null 2>&1 || \
  docker network create --subnet 172.28.0.0/16 go-admin-kit-net
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml up -d
docker compose --profile compose-fallback up -d --build
```

上述名称用于单套本地开发，不能原样用于同机隔离验收；隔离环境需独立的网络、容器名、卷、端口与密钥。停止应用使用 `make compose-down`；不要删除数据卷。

## 单服务与前端调试

先准备独立依赖和配置，再分别使用终端：

```bash
# 后端（按所需服务调整目录）
cd microservices/services/auth
go run ./cmd
```

```bash
# 前端
cd microservices/web
npm ci
npm run dev
```

默认 HMR 入口和代理配置以命令输出、`vite.config.ts` 及环境配置为准；不要把服务调试端口直接暴露到公网。

## 验证

```bash
npm ci --prefix microservices/web
bash scripts/verify-product.sh all
cd microservices
API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api
```

数据库升级、完整运行和浏览器验收见[发行验证](docs/release-validation.md)。
