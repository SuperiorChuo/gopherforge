# 工程说明

这个模板保留后台管理脚手架的基础工程能力，目标是作为新业务项目的起点。

## 协作与提交

- 提交信息要求**标题与正文均为中文**，规范见仓库根目录 `CONTRIBUTING.md` 与 `AGENTS.md`。
- 不要使用纯英文 Conventional Commits，也不要「中文标题 + 英文正文」。

## 后端边界（微服务版）

路径均在 `microservices/` 下：

- `services/*`：各业务微服务（auth、identity、system、audit、file、ai）
- `legacy-backend/cmd/main.go`：瘦后端入口（监控等兜底，**非**完整单体）
- `legacy-backend/migrations/`：默认迁移路径；`legacy-backend/docs/go_admin_kit.sql` 为手动基线参考

完整单体将位于 `monolith/server/`（阶段二），与微服务业务零调用。

## 前端边界

- **主前端**：`microservices/web/`（React + Ant Design）
- **遗留**：`tdesign-vue-go/`（Vue + TDesign，非主路径）
- 单体前端阶段二：`monolith/web/`（同一 React 技术栈，独立目录）

## 数据库

微服务栈通过 `legacy-backend/migrations/` 与各服务约定升级；Docker 后端容器启动前会幂等执行 goose 迁移。不要把运行时数据、上传文件、日志或本地数据库文件提交到仓库。

## 验证命令

```bash
cd microservices
cd legacy-backend && go test ./... && go vet ./...
cd ../web && npm run lint && npm run build
```

最近一轮稳定性、安全性和分层优化的完成情况见 `docs/development/OPTIMIZATION_STATUS.md`。
