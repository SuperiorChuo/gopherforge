# API 契约生成说明

本项目使用后端 Gin 路由生成 OpenAPI 3.1 契约，再从契约生成前端 TypeScript 类型。后端新增、删除或调整接口后，执行同一条命令即可刷新 `server/docs/openapi.json` 和 `tdesign-vue-go/src/api/generated/schema.d.ts`，减少前后端接口漂移。

## 生成命令

在项目根目录执行：

```powershell
npm run api:contract
```

该命令会依次执行：

- `npm run openapi`：运行 `server/cmd/openapi`，根据 Gin 路由表生成 `server/docs/openapi.json`。
- `npm run api:types`：运行 `scripts/generate-openapi-types.mjs`，生成前端类型声明 `tdesign-vue-go/src/api/generated/schema.d.ts`。

也可以使用 Makefile：

```powershell
make api-contract
```

## 已接入的类型能力

- `server/internal/openapi/schemas.go` 维护核心业务 schema，包括认证、用户、角色、菜单、部门、权限、字典、通知、日志、在线用户、监控详情和定时任务。
- `scripts/generate-openapi-types.mjs` 会把 OpenAPI schema 递归转换成 `components["schemas"]`、`paths` 和 `operations` 类型，并支持带值类型的 `additionalProperties`，例如 `Record<string, number>`。
- `tdesign-vue-go/src/api/generated/client.ts` 提供 `typedApi`，会从 `paths` 推导路径参数、查询参数、请求体和响应 `data` 类型。
- `tdesign-vue-go/src/api/auth.ts`、`tdesign-vue-go/src/api/system/user.ts`、`tdesign-vue-go/src/api/system/role.ts`、`tdesign-vue-go/src/api/system/menu.ts`、`tdesign-vue-go/src/api/system/department.ts`、`tdesign-vue-go/src/api/system/permission.ts`、`tdesign-vue-go/src/api/system/dict.ts`、`tdesign-vue-go/src/api/system/notice.ts`、`tdesign-vue-go/src/api/system/operationLog.ts`、`tdesign-vue-go/src/api/system/loginLog.ts`、`tdesign-vue-go/src/api/system/onlineUser.ts` 和 `tdesign-vue-go/src/api/monitor/*.ts` 已接入 `typedApi`。

## 契约测试

刷新契约后执行：

```powershell
npm run test:contract
```

当前测试会检查以下关键约束：

- `server/docs/openapi.json` 必须是 OpenAPI 3.1。
- 登录接口保持公开访问。
- 用户信息、角色详情等受保护接口必须声明 `BearerAuth`。
- 登录、用户角色分配等核心接口必须引用精细化 request/response schema。
- 部门、权限、字典、通知状态、操作日志、登录趋势、在线用户和监控详情必须引用精细化 schema。
- 前端生成类型必须包含关键路径、业务 schema 和 `BearerAuth`。
- `typedApi` 文件必须存在并引用生成 schema。

前端类型约束由 `tdesign-vue-go/src/api/generated/client.typecheck.ts` 配合 `tdesign-vue-go` 目录下的 `npm run build:type` 验证。这个文件会故意放缺少 `username` 的登录请求、缺少 `name` 的部门创建请求和错误类型的通知状态请求，并用 `@ts-expect-error` 确认 TypeScript 能拦住错误请求体。

## CI 集成

`.github/workflows/ci.yml` 的 workspace job 已加入：

```powershell
npm run api:contract
npm run test:contract
```

因此提交前建议本地先跑一遍契约生成和测试，减少 CI 上才发现接口契约漂移的情况。

## 当前边界

当前精细 schema 已覆盖后台控制台主要 CRUD、日志、监控和任务接口，前端常用 API 模块也已迁移到 `typedApi`。仍保留普通 `request` 的场景主要是文件上传、操作日志 CSV 导出等非标准 JSON 响应；后续如要进一步收敛，可以为上传表单和下载响应补充专门的类型封装。
