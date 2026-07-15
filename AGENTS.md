# 项目协作规范（人与 AI）

本文供贡献者与 AI 编码助手共同遵守。更完整的开发流程见 `CONTRIBUTING.md`。

## 语言

- 对用户可见的文档、PR 描述、Issue、**Git 提交标题与正文**默认使用**中文**。
- 代码标识符、路径、命令、API、配置键、协议名、第三方产品名可保留英文原文。

## Git 提交信息（强制）

### 总原则

1. **标题与正文都必须是中文**，禁止「中文标题 + 英文长正文」。
2. 有一定改动面时，除标题外应写正文（动机、关键点、影响）。
3. 专有名词可保留：`React`、`Ant Design`、`PostgreSQL`、`Redis`、`Traefik`、`NATS`、`OpenAPI` 等。
4. 机器可读 trailer 可保留英文格式，例如 `Co-Authored-By: Name <email@example.com>`。

### 标题格式

```text
类型（可选范围）：一句话说明
```

| 类型 | 对应旧英文 type | 用途 |
| --- | --- | --- |
| 功能 | feat | 新能力 |
| 修复 | fix | 缺陷修复 |
| 样式 | style | 纯 UI/视觉 |
| 重构 | refactor | 不改对外行为的结构调整 |
| 文档 | docs | 文档 |
| 测试 | test | 测试 |
| 杂项 | chore | 工具链/杂项 |
| 性能 | perf | 性能 |
| 持续集成 | ci | CI/CD |
| 合并 | merge | 合并 |
| 构建 | build | 构建 |
| 回滚 | revert | 回滚 |

范围写中文，例如：`（React Ant Design 前端）`、`（服务端）`、`（认证服务）`。

### 正文写法

- 用完整中文句子或中文列表说明「为什么」和「改了什么」。
- 避免整段英文实现说明、英文 bullet 列表。
- 单行小修可只写标题；跨模块、视觉体系、微服务拆分等应写正文。

### 正例

```text
样式（React Ant Design 前端）：标签页光条、玻璃图片预览与呼吸标志

- 标签页指示条改为发光渐变光带（与选择器激活条同一套语言）
- 图片预览（文件页）：毛玻璃遮罩，操作栏变为带边缘光的玻璃胶囊
- 侧栏标志徽章每 5 秒缓慢呼吸发光——站点「心跳」；在偏好减少动效下关闭
- 数字输入步进器获得染色玻璃悬停效果

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
```

### 反例

```text
feat: remake tabs ink bar

Tabs ink bar becomes a glowing gradient strip...
```

```text
样式（React Ant Design 前端）：标签页光条、玻璃图片预览与呼吸标志

- Tabs ink bar becomes a glowing gradient strip
- Image preview: frosted mask...
```

## 代码改动原则

- 保持模板通用性，避免塞入与脚手架无关的业务耦合。
- 接口、权限码、菜单种子、OpenAPI、迁移脚本变更需同步。
- 不提交密钥、`.env`、本地数据、构建产物、上传文件。
- 优先小步可审 PR；验证命令见 `CONTRIBUTING.md`。

## 架构速览（双产品线）

本仓库是 **monorepo 外壳**，内含两个**互不调用**的产品：

| 路径 | 说明 |
|------|------|
| `microservices/` | 微服务：多服务 + 网关 + React 前端 |
| `microservices/services/*` | auth / identity / system / audit / file / ai / **monitor** |
| `microservices/web/` | React + Ant Design（微服务前端） |
| `monolith/` | 单体：`server/` + `web/`（与微服务零调用） |
| `platform/` | 公共监控模板等 |
| `docs/` | 工程文档与 [`PRODUCT_LINES.md`](docs/PRODUCT_LINES.md) |

开发时只进入其中一条产品线；禁止跨线业务依赖。
