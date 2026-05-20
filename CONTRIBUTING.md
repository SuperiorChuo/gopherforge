# 贡献指南

感谢你愿意参与 Go Admin Kit。这个项目定位是干净、可复用、便于二次开发的 Go + Vue 后台管理脚手架，贡献时请优先保持模板的通用性。

## 开发环境

- Go 1.26.3+
- Node.js 20.19+ 或 22.12+，推荐 Node.js 24
- npm
- Docker Desktop
- uv 0.11+

Python 辅助工具统一使用项目内隔离环境，不要向全局 Python 安装依赖：

```powershell
uv sync
uv run python --version
```

## 本地启动

```powershell
Copy-Item .env.example .env
docker compose up -d --build
```

默认地址：

- 前端：`http://localhost:3000`
- 后端：`http://localhost:8081`
- 健康检查：`http://localhost:8081/api/v1/health/ready`

默认管理员账号仅用于本地开发：

- 用户名：`admin`
- 密码：`admin123`

## 分支和提交

建议从 `main` 创建短分支：

```powershell
git checkout -b feat/example-change
```

提交信息建议使用 Conventional Commits：

- `feat: add example feature`
- `fix: correct login redirect`
- `docs: improve quick start`
- `test: cover permission fallback`
- `chore: update tooling`

## 代码约定

- 后端接口、权限码、菜单种子和 OpenAPI 契约需要同步更新。
- 前端页面优先沿用现有 TDesign 组件、布局和 API client 模式。
- 新增数据库结构优先使用 `server/migrations/`，并确认基线 SQL 或迁移说明同步。
- 不提交本地运行数据、日志、上传文件、数据库卷、`.env`、构建产物和密钥。
- 文档正文默认使用中文，命令、路径、API、配置项和包名保持原文。

## 提交前验证

后端：

```powershell
cd server
go test ./...
go vet ./...
```

前端：

```powershell
cd tdesign-vue-go
npm run test
npm run build:type
npm run lint
npm run stylelint
npm run build
npm audit --omit=dev
```

完整栈启动后可以执行：

```powershell
npm run test:smoke:unit
npm run smoke:api
npm run e2e:frontend
```

API 契约：

```powershell
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
