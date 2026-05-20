# 发布前检查清单

## 配置

- [ ] 已替换 `JWT_SECRET`，长度不少于 32 个字符。
- [ ] 已替换 MySQL、Redis、MinIO、Grafana 等默认密码。
- [ ] `APP_ENV=production` 前已通过后端生产配置校验。
- [ ] `CORS_ALLOW_ORIGINS` 只包含可信前端域名。
- [ ] 上传存储路径或对象存储 bucket 已确认。

## 数据库

- [ ] 新环境已导入 `server/docs/go_admin_kit.sql`。
- [ ] 默认管理员密码已修改。
- [ ] 新增业务表已有初始化或迁移策略。
- [ ] 本地测试数据、日志数据和上传文件未进入发布包。

## 后端

- [ ] `go test ./...` 通过。
- [ ] `go vet ./...` 通过或已确认剩余提示。
- [ ] 健康检查 `/api/v1/health/ready` 可访问。
- [ ] 关键接口已验证鉴权和权限码。
- [ ] 日志、审计和请求 ID 能正常记录。

## 前端

- [ ] `npm run build:type` 通过。
- [ ] `npm run e2e:frontend` 通过，桌面和移动端登录链路正常。
- [ ] `npm run build` 通过。
- [ ] 登录、仪表盘、系统管理和监控页面可访问。
- [ ] 菜单、路由和后端权限种子一致。
- [ ] 构建产物不包含本地敏感配置。

## Docker

- [ ] `docker compose up -d --build` 可启动完整服务。
- [ ] MySQL 和 Redis healthcheck 正常。
- [ ] 后端容器日志没有启动错误。
- [ ] 前端容器能代理或访问后端 API。
