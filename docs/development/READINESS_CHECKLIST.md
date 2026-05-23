# 发布前检查清单

> 最近一次自动化复核：2026-05-21。以下命令和 Docker 烟测已在本机完成；涉及真实生产密钥、域名、默认账号和目标环境数据的项目仍需上线前人工确认。

## 自动化验证已完成

### 后端与契约

- [x] `cd server; go test ./...` 通过。
- [x] `cd server; go vet ./...` 通过。
- [x] `cd server; go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --config ../.golangci.yml ./...` 通过，`0 issues`。
- [x] `npm run api:contract` 通过，且 `git diff --exit-code -- server/docs/openapi.json tdesign-vue-go/src/api/generated/schema.d.ts` 无漂移。
- [x] `npm run test:contract` 通过，5 个契约测试全部通过。
- [x] `npm run test:smoke:unit` 通过，7 个 API smoke 单元测试全部通过。
- [x] 本机 `/api/v1/health/ready` 可访问，数据库和 Redis 均返回 `status=ok`。

### 前端

- [x] `cd tdesign-vue-go; npm run test` 通过，7 个测试文件、20 个测试全部通过。
- [x] `cd tdesign-vue-go; npm run build:type` 通过。
- [x] `cd tdesign-vue-go; npm run lint` 通过。
- [x] `cd tdesign-vue-go; npm run stylelint` 通过。
- [x] `cd tdesign-vue-go; npm run build` 通过。
- [x] `cd tdesign-vue-go; npm run e2e:frontend` 通过，桌面和移动端共 4 个用例全部通过；前端 E2E 使用 API mock 验证登录 UI、验证码交互、鉴权请求和路由跳转，真实后端登录链路由 API smoke 覆盖。
- [x] 构建产物已扫描 `JWT_SECRET`、`PASSWORD=`、`SECRET_KEY`、`localhost:3306`、`redis://`，未发现敏感配置；`root:` 仅来自 vendor 构建代码片段。

### Docker 本机烟测

- [x] `docker compose config` 通过。
- [x] 默认宿主机端口被现有容器占用时，使用 `MYSQL_PORT=13306`、`REDIS_PORT=16379`、`MINIO_API_PORT=19000`、`MINIO_CONSOLE_PORT=19001`、`BACKEND_PORT=18081`、`FRONTEND_PORT=13000` 完成端口避让；不再需要的冲突容器已通过 `docker stop <container-name>` 停止或确认可保留并行运行。
- [x] MySQL、Redis 和 backend healthcheck 均为 healthy。
- [x] 后端容器日志显示 goose 迁移成功、数据库连接成功、Redis 连接成功、服务启动成功，未发现 fatal/panic/error。
- [x] `http://localhost:18081/api/v1/health/ready` 返回 HTTP 200，数据库和 Redis 均为 `ok`。
- [x] `http://localhost:13000/` 返回 HTTP 200，并加载构建后的前端静态页面。
- [x] 本机 `scaffold-minio` 健康时，可在 `server/` 下执行 `make smoke-storage-minio` 完成对象存储上传、读取、缺失对象识别和删除清理 smoke。
- [x] 烟测结束已执行 `docker compose down` 清理本次容器和网络，未删除 volumes。

### 对象存储 smoke 入口

默认命令使用本机 scaffold MinIO，不依赖真实云 S3：

```powershell
cd server
make smoke-storage-minio
```

该目标会通过 `docker exec scaffold-minio` 调用容器内 `mc mb -p` 确保 `go-admin-kit` bucket 存在，然后以 S3-compatible 配置运行 `go test ./internal/pkg/upload -run TestObjectStorageSmokeOpenReadsRealEndpoint -count=1 -v`。默认注入的环境变量为：

- `BLACK8_OBJECT_STORAGE_SMOKE=1`
- `BLACK8_OBJECT_STORAGE_PROVIDER=s3`
- `BLACK8_OBJECT_STORAGE_ENDPOINT=127.0.0.1:9000`
- `BLACK8_OBJECT_STORAGE_BUCKET=go-admin-kit`
- `BLACK8_OBJECT_STORAGE_REGION=us-east-1`
- `BLACK8_OBJECT_STORAGE_ACCESS_KEY=minioadmin`
- `BLACK8_OBJECT_STORAGE_SECRET_KEY=minioadmin`
- `BLACK8_OBJECT_STORAGE_USE_SSL=false`
- `BLACK8_OBJECT_STORAGE_OBJECT_PREFIX=black8-smoke`

CI 或非默认本地环境可覆盖 Make 变量，例如 `OBJECT_STORAGE_SMOKE_ENDPOINT`、`OBJECT_STORAGE_SMOKE_BUCKET`、`OBJECT_STORAGE_SMOKE_ACCESS_KEY`、`OBJECT_STORAGE_SMOKE_SECRET_KEY`。如果 bucket 已由 CI 预置且没有 Docker 容器，可设置 `OBJECT_STORAGE_SMOKE_CREATE_BUCKET=0`，但仍需保证 endpoint、bucket 和凭据指向临时 MinIO/S3-compatible 服务。

## 生产环境上线前待确认

### 配置

- [ ] 已替换 `JWT_SECRET`，长度不少于 32 个字符。
- [ ] 已替换 MySQL、Redis、MinIO、Grafana 等默认密码。
- [ ] `APP_ENV=production` 前已通过目标环境后端生产配置校验。
- [ ] `CORS_ALLOW_ORIGINS` 只包含可信前端域名。
- [ ] 上传存储路径或对象存储 bucket 已在目标环境确认。
- [ ] WebSocket 通知的生产前端域名已加入 `CORS_ALLOW_ORIGINS`，反向代理已透传 `Upgrade` 和 `Connection` 等 WebSocket upgrade 头。

### 数据库

- [ ] 目标环境已执行 `server/migrations/` 下的 goose 迁移；仅手动初始化路径需要导入 `server/docs/go_admin_kit.sql`。
- [ ] 目标环境或同版本副本已完成 migration rehearsal，确认 goose 迁移可重复执行且无未预期 schema 变更。
- [ ] 默认管理员密码已在目标环境修改。
- [ ] 新增业务表在目标环境已有初始化或迁移策略。
- [ ] 本地测试数据、日志数据和上传文件未进入发布包。

### 业务链路

- [ ] 目标环境关键接口已验证鉴权和权限码。
- [ ] 目标环境日志、审计和请求 ID 能正常记录。
- [ ] 目标环境登录、仪表盘、系统管理和监控页面可访问。
- [ ] 目标环境已验证 `POST /api/v1/ws/notifications/ticket` 可获取 ticket，且 `GET /api/v1/ws/notifications?ticket=...` 可通过正式域名完成 WebSocket 连接。
- [ ] 目标环境对象存储已完成上传、下载或预览、删除 smoke，确认 bucket、凭据、公开 URL 和代理路径一致。
- [ ] 目标环境菜单、路由和后端权限种子一致。
- [ ] 目标环境前端容器能通过正式域名或网关访问后端 API。
