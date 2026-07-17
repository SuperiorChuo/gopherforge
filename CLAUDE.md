# 项目规范（Claude 工作约定）

## 代码完成后的部署规则

**每次写完代码并通过 build/lint 验证后，必须热更新到内网开发服务器（192.168.220.109），规则：**

- **前端改动（microservices/web）：每次必部**。流程：
  1. `./scripts/dev-sync.sh once` 推送源码
  2. `ssh root@192.168.220.109 "cd /www/go-admin-kit/src/microservices && export COMPOSE_PROJECT_NAME=go-admin-kit && docker compose up -d --build frontend"`
  3. 验证 `http://192.168.220.109:18100` 返回新构建（对比 assets 哈希或 grep 新增类名）
- **后端改动：按需部署**。只有当用户要测试该功能、或前端改动依赖新后端接口时才重建对应服务（如 `system-service`、`auth-service`、`im-service`），不要顺手全量重建。

## 其他约定

- 禁止在本机 Mac 上启动 Docker；运行时验证一律走上面的远程热更新流程（详见 docs/remote-dev.md）。
- 本机只跑不依赖 Docker 的工具链：`go test ./...`、`go build`、前端 `npm run build`、`npm run lint`（oxlint）。
- 前端构建命令：`cd microservices/web && npm run build`（tsc -b + vite build），提交前 build 和 lint 必须通过。
- 端口速查：网关 18100（主入口）、前端静态 13100、Vite HMR 13200、PG 15434、Redis 16380。
