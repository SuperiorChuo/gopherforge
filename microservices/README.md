# 微服务版（Go Admin Kit Microservices）

本目录是**可独立运行的微服务产品线**，与仓库内 `monolith/` 单体版互不调用业务接口。

## 包含内容

| 路径 | 说明 |
|------|------|
| `services/*` | auth / identity / system / audit / file / ai |
| `legacy-backend/` | 拆分后的瘦后端（监控等兜底，**不是**完整单体） |
| `web/` | React + Ant Design 前端（主前端） |
| `docker-compose.yml` | 网关 + 依赖 + 各服务 + 前端 |
| `go.work` | 仅本线 Go 模块 |
| `tests/` / `scripts/` | 冒烟、契约与辅助脚本 |

## 快速启动

在**本目录**执行：

```bash
cp .env.example .env   # 若尚无 .env
docker compose up -d --build
```

默认：

- 网关：`http://localhost:8000`
- 前端容器：`http://localhost:3000`（也经网关 `/` 进入）
- 本地开发默认管理员：`admin` / `admin123`（仅开发）

推荐经**网关**访问 API。

## 本地开发（可选）

```bash
# 依赖
docker compose up -d go-admin-kit-postgres go-admin-kit-redis go-admin-kit-nats

# 某服务
cd services/auth && go run ./cmd

# 前端
cd web && npm ci && npm run dev
```

根目录 `make compose-up` 会转发到本目录。

## 验证

```bash
npm run test:smoke:unit
npm run test:contract
# 栈起来后：
API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api
```

## 说明

- 前端技术栈统一为 React Ant Design（`web/`）。
- 仓库根 `tdesign-vue-go/` 为**遗留**前端，非本产品线主路径。
- 完整单体请见 `../monolith/`（阶段二，当前仅占位）。
