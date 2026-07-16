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

## 远程热更新部署（强制）

> 内网开发机 `192.168.220.109`。详细说明见 [`docs/remote-dev.md`](docs/remote-dev.md)。  
> **策略：只部署 monorepo 微服务，不部署单体。**

### 总原则

1. **凡改动了会影响远端运行效果的代码，任务收尾前必须热更新部署**，不要只改本地就结束。
2. 纯文档 / 仅 `AGENTS.md` 规范 / 用户明确说「不要部署」时，可跳过。
3. 部署后应用一两句中文说明访问入口与是否成功（如 13100/13200/18100）。

### 标准流程（按改动面选）

| 改动范围 | 必须执行 |
| --- | --- |
| 任意需上机的代码 | `./scripts/dev-sync.sh once` |
| `microservices/web/**` 前端 | 同步后 **重建静态前端**（用户主入口 **13100**）：见下方命令 |
| 单微服务 Go 代码 | 同步后重建对应服务：`docker compose up -d --build <service>` |
| 多服务 / compose / 网关 / 环境变量 | `./scripts/remote-ms-deploy.sh` 或按服务逐个 `--build` |
| 仅文档/规范 | 可不部署 |

### 推荐命令（本机仓库根目录）

```bash
# 1) 始终先推源码
./scripts/dev-sync.sh once

# 2a) 微服务前端 → 静态 13100（用户默认看这个）
ssh -i "$HOME/.ssh/id_ed25519" -o IdentitiesOnly=yes -o BatchMode=yes root@192.168.220.109 \
  'cd /www/go-admin-kit/src/microservices && export COMPOSE_PROJECT_NAME=go-admin-kit && docker compose up -d --build frontend'

# 2b) 单后端服务示例（identity）
ssh -i "$HOME/.ssh/id_ed25519" -o IdentitiesOnly=yes -o BatchMode=yes root@192.168.220.109 \
  'cd /www/go-admin-kit/src/microservices && export COMPOSE_PROJECT_NAME=go-admin-kit && docker compose up -d --build identity-service'

# 2c) 全量微服务栈
./scripts/remote-ms-deploy.sh
```

### 访问入口

| URL | 用途 |
| --- | --- |
| http://192.168.220.109:13100 | **静态前端（默认验收）** |
| http://192.168.220.109:13200 | Vite HMR（dev-sync 后源码热更；`gak-ms-web-dev`） |
| http://192.168.220.109:18100 | Traefik 网关（API + 路由） |

### AI 执行注意

- SSH 密钥默认 `~/.ssh/id_ed25519`，目标 `root@192.168.220.109`；**不要把密码写进仓库或对话日志。**
- `docker compose up -d --build frontend` 可能数分钟，需足够超时；完成后 `curl` 或 `docker compose ps` 确认。
- 前端改完：**13100 必须重建**，不要只靠 13200 就当作「已部署」。
- 部署失败时说明原因并重试；网络不在内网时告知用户，勿假装已上线。

## 架构速览（一人 monorepo）

本仓库是 **单人维护的 monorepo**，内含多条产品线（目录分开、进程可分开部署）：

| 路径 | 说明 |
|------|------|
| `microservices/` | 微服务中台：多服务 + 网关 + React |
| `microservices/services/*` | auth / identity / system / audit / file / ai / im / **monitor** |
| `microservices/web/` | React + Ant Design（微服务前端） |
| `monolith/` | 单体：`server/` + `web/`（与微服务业务零调用） |
| `freeswitch-cc/` | 呼叫媒体：FreeSWITCH + control-api；**中台可控制 FS**，媒体独立 compose |
| `platform/` | 公共监控模板等 |
| `docs/` | 工程文档与 [`PRODUCT_LINES.md`](docs/PRODUCT_LINES.md) |

- 单体 与 微服务：禁止互相依赖业务代码。  
- 呼叫：允许中台经 HTTP/Webhook **控制** `freeswitch-cc`；不要把 FS 编进业务微服务 binary。
