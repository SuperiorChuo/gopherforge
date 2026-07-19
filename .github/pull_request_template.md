## 变更说明

请说明这次 PR 解决的问题、主要改动和影响范围。涉及产品线时标明 **微服务** / **单体** / **公共**。

## 变更类型

- [ ] 功能新增
- [ ] Bug 修复
- [ ] 文档更新
- [ ] 测试补充
- [ ] 工程配置或依赖更新
- [ ] 重构或性能优化

## 验证

按改动线勾选：

**微服务（`microservices/`）**

- [ ] `cd services/monitor && go test ./... && go vet ./...`（及相关服务）
- [ ] `cd web && npm run lint && npm run build`
- [ ] `npm run test:smoke:unit` / `npm run test:contract`
- [ ] `npm run api:contract` 且 `git diff --exit-code -- services/monitor/docs/openapi.json`
- [ ] 栈已启动时：`API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api`

**单体（`monolith/`）**

- [ ] `cd server && go test ./... && go vet ./...`
- [ ] `cd web && npm run lint && npm run build`

## 数据库、配置和安全影响

- [ ] 不涉及数据库变更
- [ ] 不涉及配置项变更
- [ ] 不涉及权限码或菜单种子
- [ ] 不涉及认证、授权、上传或安全策略

如有涉及，请说明迁移方式、兼容性和回滚方式。

## 边界确认

- [ ] 未引入 monolith ↔ microservices 的业务依赖或互相调用

## 截图

涉及 UI 时请附上截图，或说明无需截图的原因。
