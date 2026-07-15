# 贡献指南

感谢你愿意参与 Go Admin Kit。这个项目定位是干净、可复用、便于二次开发的 Go + Vue 后台管理脚手架，贡献时请优先保持模板的通用性。

## 开发环境

- Go 1.26.3+
- Node.js 20.19+ 或 22.12+，推荐 Node.js 24
- npm
- Docker Desktop
- uv 0.11+

本仓库含 **微服务**（`microservices/`，当前可运行）与 **单体**（`monolith/`，规划中）两条独立产品线，业务互不调用。日常开发请进入 `microservices/`。

Python 辅助工具统一使用项目内隔离环境，不要向全局 Python 安装依赖：

```powershell
uv sync
uv run python --version
```

## 本地启动（微服务）

```bash
cd microservices
cp .env.example .env
docker compose up -d --build
```

默认地址：

- 网关：`http://localhost:8000`
- 前端：`http://localhost:3000`
- 健康检查：`http://localhost:8000/api/v1/health/ready`

默认管理员账号仅用于本地开发：

- 用户名：`admin`
- 密码：`admin123`

## 分支和提交

建议从 `main` 创建短分支：

```powershell
git checkout -b feat/example-change
```

### 提交信息规范（全中文）

本仓库要求 **提交标题与正文均使用中文**。不要只写中文标题、英文正文。

#### 标题格式

```text
类型（可选范围）：一句话说明
```

| 类型 | 含义 |
| --- | --- |
| `功能` | 新能力 |
| `修复` | 缺陷修复 |
| `样式` | 纯 UI/视觉，不改行为逻辑 |
| `重构` | 结构整理，不改对外行为 |
| `文档` | 文档与说明 |
| `测试` | 测试补充或调整 |
| `杂项` | 工具链、依赖、杂项 |
| `性能` | 性能优化 |
| `持续集成` | CI/CD |
| `合并` | 合并类提交 |
| `构建` | 构建脚本/产物相关 |
| `回滚` | 回滚变更 |

范围示例：`（React Ant Design 前端）`、`（服务端）`、`（认证服务）`。无明确模块时可省略范围。

#### 正文要求

- 有一定改动面时，**必须写正文**：说明动机、关键改动点、影响范围；可用列表。
- 正文必须是中文。专有名词可保留原文，如 `React`、`PostgreSQL`、`Redis`、`Traefik`、`NATS`。
- 不要用纯英文段落描述实现细节。
- Git 机器可读 trailer 可保留英文格式，例如：
  `Co-Authored-By: Name <email@example.com>`

#### 示例

```text
功能：拆出认证服务微服务，接入 Traefik 网关与 NATS 事件总线

微服务拆分第一、二阶段。Traefik 成为统一入口，后续新服务只需加标签即可接入。

- 认证服务在 8082 端口提供登录/刷新/登出等路由
- 网关层转发认证校验令牌，无效令牌到不了单体
```

```text
样式（React Ant Design 前端）：标签页光条、玻璃图片预览与呼吸标志

- 标签页指示条改为发光渐变光带（与选择器激活条同一套语言）
- 图片预览：毛玻璃遮罩，操作栏变为带边缘光的玻璃胶囊
- 侧栏标志徽章每 5 秒缓慢呼吸发光；偏好减少动效时关闭
```

```text
修复（React Ant Design 前端）：修复切换天数时登录趋势图抖动

三层原因：标签行空高塌陷、布局过早按天数重排、切换无过渡遮罩。
现改为固定行高、按已加载数据回流，并加短时雾化过渡。
```

反例（不要这样）：

```text
# 错误：英文 Conventional Commits
feat: add auth service

# 错误：仅标题中文，正文仍是英文
样式（React Ant Design 前端）：标签页光条
Tabs ink bar becomes a glowing gradient strip...
```

更完整的约定见仓库根目录 `AGENTS.md`（供人与 AI 编码助手共同遵守）。

## 代码约定

- 后端接口、权限码、菜单种子和 OpenAPI 契约需要同步更新。
- 微服务前端使用 `microservices/web`（React + Ant Design）；`tdesign-vue-go/` 为遗留，非主路径。
- 新增数据库结构优先使用 `microservices/legacy-backend/migrations/`（及各服务自有迁移约定），并确认说明同步。
- 不提交本地运行数据、日志、上传文件、数据库卷、`.env`、构建产物和密钥。
- 文档正文默认使用中文；命令、路径、API、配置项和包名可保持原文。

## 提交前验证

在 `microservices/` 下：

```bash
cd microservices
cd legacy-backend && go test ./... && go vet ./...
cd ../services/auth && go test ./...
cd ../../web && npm run lint && npm run build
cd .. && npm run test:smoke:unit && npm run test:contract
```

完整栈启动后：

```bash
cd microservices
API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api
```

API 契约：

```bash
cd microservices
npm run api:contract
npm run test:contract
```

## Pull Request

发起 PR 前请确认：

- 变更范围聚焦，没有混入无关格式化。
- 已说明变更原因、影响范围和验证命令。
- 涉及 UI 的变更附上截图或说明。
- 涉及配置、数据库、权限或迁移的变更已在 PR 中标注。
- 公开仓库中没有提交真实密钥、账号、生产地址或内部数据。
