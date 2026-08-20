---
description: GopherForge 网关调用约定、认证与分页规则、各模块接口入口，以及 OpenAPI 3.1 契约覆盖范围。
---

# API 参考

GopherForge 的 API 全部走 **Traefik 网关统一入口**，形态一致、约定统一。本页是总入口：调用总则 + 各模块接口速查 + 机器可读契约。

## 调用总则

| 约定 | 说明 |
|------|------|
| 入口 | 一律经网关：`http://<网关>/api/v1/...`（本地默认 `localhost:8000`），不要直连服务端口 |
| 认证 | `Authorization: Bearer <access_token>`；登录 `POST /api/v1/login`（用户名/密码/验证码，租户码可选），刷新走 refresh token 轮转 |
| 鉴权 | 网关 ForwardAuth 校验后注入 `X-Auth-*` 头；接口级权限由各服务按权限码中间件校验（bpm 例外：Bearer JWT 自校验） |
| 响应信封 | 统一 `{ "code": 200, "message": "success", "data": ... }`；列表类 `data` 为 `{ list, total }` |
| 分页 | `page` / `page_size` 查询参数（默认 1 / 10） |
| 错误码 | `code` 非 200 即业务错误；对外文案可在「错误码管理」在线热改 |

## 各模块接口速查

模块文档里维护着**带权限码的端点表**（从路由注册代码提取），按域查最快：

| 模块 | 覆盖 |
|------|------|
| [RBAC 权限体系](/modules/rbac) | 用户 / 角色 / 权限 / 菜单 / 部门 / 岗位 + Excel 导入导出 |
| [多租户与套餐](/modules/tenant) | 租户 / 租户套餐 |
| [审批流 BPM](/modules/bpm) | 流程定义 / 实例 / 任务 / 抄送 / internal 业务接入 |
| [代码生成器](/modules/codegen) | 表反射 / 预览 / 下载 |
| [审计日志](/modules/audit) | 操作日志 / 登录日志 / 业务审计日志 |
| [认证与安全](/modules/auth) | 登录 / 刷新 / TOTP / OAuth / OAuth2 服务端 |

## 机器可读契约（OpenAPI 3.1）

- **在线浏览**：[monitor 服务契约（交互式）](https://superiorchuo.github.io/gopherforge/docs/api-reference.html)——直读仓库 main 分支的契约文件，永远与代码同步。
- 契约源文件：[`microservices/services/monitor/docs/openapi.json`](https://github.com/SuperiorChuo/gopherforge/blob/main/microservices/services/monitor/docs/openapi.json)，由代码生成（`make api-contract` = 生成契约 + 生成前端 TS 类型）。
- **CI 漂移门禁**：契约与代码不一致时 CI 直接红——契约不是文档摆设，是被强制执行的约定。

::: tip 覆盖说明（如实）
机器可读契约目前覆盖 **monitor 服务 + common 通用端点**（契约工具链的首个落地服务）；其余服务的接口以上表各模块页的端点速查为准，契约化会随工具链逐服务推进。
:::
