# 工程说明

这个模板保留后台管理脚手架的基础工程能力，目标是作为新业务项目的起点。

## 后端边界

- `server/cmd/main.go` 是 API 服务入口。
- `server/internal/api/` 放 HTTP handler 和路由注册。
- `server/internal/service/` 放业务服务。
- `server/internal/dao/` 放数据访问封装。
- `server/internal/model/` 放数据库模型和请求响应模型。
- `server/internal/pkg/` 放可复用基础能力。
- `server/migrations/` 是默认数据库初始化和升级路径，`server/docs/go_admin_kit.sql` 仅保留为手动基线。

当前后端只保留认证、RBAC、系统管理、文件、字典、通知、日志、监控和健康检查能力。

## 前端边界

- `tdesign-vue-go/src/pages/` 放页面。
- `tdesign-vue-go/src/api/` 放 Web API client。
- `tdesign-vue-go/src/router/` 放路由定义。
- `tdesign-vue-go/src/components/` 放通用组件。
- `tdesign-vue-go/src/locales/` 放国际化文案。

新增页面时优先沿用现有 TDesign 页面结构、API client 和路由模块组织方式。

## 数据库

模板默认通过 `server/migrations/` 初始化和升级数据库，Docker 后端容器会在主服务启动前幂等执行 goose 迁移；`server/docs/go_admin_kit.sql` 仅作为手动基线保留。新项目可以在此基础上选择：

- 继续维护 goose 迁移链，并在必要时同步单文件基线 SQL。
- 如迁移链过长，可在发布大版本时重建基线迁移。

不要把运行时数据、上传文件、日志或本地数据库文件提交到模板目录。

## 验证命令

```powershell
cd server
go test ./...
go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --config ../.golangci.yml ./...

cd ..\tdesign-vue-go
npm run test
npm run build:type
npm run lint
npm run stylelint
npm run build
```

在修改路由、权限、菜单或数据库结构后，需要同步检查：

- `server/internal/api/routes.go`
- `server/internal/service/system/menu_seed.go`
- `server/migrations/`
- `server/docs/go_admin_kit.sql`
- `tdesign-vue-go/src/router/`
- `tdesign-vue-go/src/api/`

最近一轮稳定性、安全性和分层优化的完成情况见 `docs/development/OPTIMIZATION_STATUS.md`。
