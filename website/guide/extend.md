# 二次开发：加一个业务服务

GopherForge 的扩展原则：**底座保持通用，业务以「新微服务 + 网关标签」接入**，不把脚手架改成巨石。本文以加一个 `demo` 业务服务为例走一遍全流程。

## 第 0 步：先试试代码生成器

如果你的业务是标准 CRUD，不用手写——控制台「系统管理 → 代码生成器」选表配字段，一键生成前后端代码（支持单表/树表/主子表，见[代码生成器](/modules/codegen)），下载后按下文接线即可。

## 第 1 步：新建服务

```bash
cd microservices/services
mkdir demo && cd demo
go mod init github.com/go-admin-kit/services/demo
```

参考 `bpm` 服务的结构（它是最新的自包含范本）：

```text
demo/
├── cmd/main.go            # 入口：配置、DB、路由、优雅退出
├── internal/
│   ├── api/               # gin handler + 路由注册
│   ├── model/             # GORM 模型（含 tenant_id）
│   ├── store/             # 持久层
│   └── config/            # 纯环境变量配置
└── Dockerfile
```

把模块加进仓库根的 `go.work`。

## 第 2 步：接入网关

在 `docker-compose.yml` 补服务块 + Traefik 标签：

```yaml
demo-service:
  build: { context: ./services/demo, dockerfile: Dockerfile }
  labels:
    traefik.enable: "true"
    traefik.http.routers.demo.rule: "Path(`/api/v1/demo`) || PathPrefix(`/api/v1/demo/`)"
    traefik.http.routers.demo.middlewares: "auth-verify@docker"   # 挂 ForwardAuth
    traefik.http.services.demo.loadbalancer.server.port: "8097"
```

::: warning 最常见的坑：路由规则不含新路径 → 经网关 404
Traefik 用**显式路径列表**路由（exact `Path()` + 子树 `PathPrefix()`）。服务里新增了顶级路径（如 `/api/v1/demo-reports`）却没同步 router rule，请求会落到 monitor 的兜底路由返回 404。改服务路由时必查 compose 标签。
:::

## 第 3 步：鉴权与契约

- handler 只信任网关注入的 `X-Auth-User-ID` / `X-Auth-Tenant-ID` / `X-Auth-Platform-Admin` 头（内网直连场景 Bearer JWT 兜底）。
- 响应统一 `{code, message, data}` 信封，分页参数 `page` / `page_size`，返回 `{list, total}`（前端 Axios 拦截器按此解包，见 [API_CONTRACT](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/development/API_CONTRACT.md)）。
- 权限码约定 `{domain}:{resource}:{action}`（如 `demo:order:list`），路由上挂权限中间件，权限点用迁移播种。
- 服务间内部端点走 `X-Internal-Token`，未配置密钥直接 503。

## 第 4 步：数据与迁移

- 模型统一带 `tenant_id`（`not null;default:1;index`），金额存**分**（int64）。
- 核心域表结构改动写 goose 迁移放 `services/monitor/migrations/`（共享真源）；实验线服务可 AutoMigrate 自管。
- 权限点、菜单从属：权限走 SQL 迁移，菜单条目加进 system 服务的 `menu_seed.go`。

## 第 5 步：前端

1. `web/src/api/demo.ts` 写接口封装（照 `api/bpm.ts` 惯例）。
2. `web/src/pages/demo/` 加页面（列表页三件套：TableToolbar + 过滤表单 + Table）。
3. `web/src/router/index.tsx` 补路由，菜单种子补条目（页面按菜单权限显隐，`usePermission().hasPerm(code)` 控制按钮级）。

## 第 6 步：验证清单

```bash
(cd services/demo && go test ./... && go vet ./...)
(cd web && npm run lint && npm run build)
docker compose up -d --build demo-service
curl -s localhost:8000/api/v1/demo/health/ready   # 若有独立健康端点
```

提交规范：**标题与正文全中文**、Conventional 风格（`功能：` / `修复：` 前缀），见 CONTRIBUTING.md。

## 想挂审批？

业务对象要走审批（如订单核准），不需要自己写流程：调 bpm 的 internal 发起端点 + 接终态回调即可，详见[审批流 · 业务接入](/modules/bpm#业务表单模式接入)。
