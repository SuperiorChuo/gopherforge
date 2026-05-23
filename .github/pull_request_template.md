## 变更说明

请说明这次 PR 解决的问题、主要改动和影响范围。

## 变更类型

- [ ] 功能新增
- [ ] Bug 修复
- [ ] 文档更新
- [ ] 测试补充
- [ ] 工程配置或依赖更新
- [ ] 重构或性能优化

## 验证

请勾选已执行的命令，并补充必要输出摘要。

- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] `npm run test`
- [ ] `npm run build:type`
- [ ] `npm run lint`
- [ ] `npm run stylelint`
- [ ] `npm run build`
- [ ] `npm run e2e:frontend`
- [ ] `npm run api:contract`
- [ ] `git diff --exit-code -- server/docs/openapi.json tdesign-vue-go/src/api/generated/schema.d.ts`
- [ ] `npm run test:contract`
- [ ] `npm run smoke:api`

## 数据库、配置和安全影响

- [ ] 不涉及数据库变更
- [ ] 不涉及配置项变更
- [ ] 不涉及权限码或菜单种子
- [ ] 不涉及认证、授权、上传或安全策略

如有涉及，请在这里说明迁移方式、兼容性和回滚方式。

## 发布前专项检查

- [ ] 数据库变更已完成 migration rehearsal，并记录目标库或同版本副本的验证结果。
- [ ] OpenAPI 与前端生成类型无漂移，`server/docs/openapi.json` 和 `tdesign-vue-go/src/api/generated/schema.d.ts` 已随接口变更提交。
- [ ] WebSocket 通知已验证 ticket 获取、`GET /api/v1/ws/notifications?ticket=...` 连接、反向代理 upgrade 和 `Origin`/`CORS_ALLOW_ORIGINS` 配置。
- [ ] 对象存储已完成上传、下载或预览、删除 smoke，并确认 bucket、凭据、公开 URL 和代理路径。

## 截图

涉及 UI 时请附上桌面端和移动端截图，或说明无需截图的原因。
