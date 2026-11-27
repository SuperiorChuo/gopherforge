# shared/pkg 目录分类清单

> 更新日期：2026-08-14
>
> 本清单只表达职责和消费者边界，不把已有独立 Go package 再机械包进新的总目录。包路径本身是 import 边界；涉及 authz 的消费者正在并行收敛批次中，本批不改其实现。

## 分类

| 类别 | 包 | 归属说明 |
|---|---|---|
| 运行基础 | `connpool`、`graceful`、`logger`、`metrics`、`middleware`、`response`、`pagination`、`errors`、`health`、`redis`、`database`、`captcha`、`cache`、`runtimeconfig` | 服务启动、HTTP 响应、分页、错误、健康检查、Redis、Postgres、验证码、缓存和运行时配置失效等通用运行能力 |
| 配置与安全 | `envsecret`、`secretstrength`、`mask`、`secretbox`、`jwt`、`consoleauth`、`authdao`、`tenant`、`tenantctx`、`authz` | 配置密钥、凭据、身份会话、租户上下文和数据权限；`authz`/`tenantctx` 的服务消费者由 authz 收敛批次负责 |
| 服务间通信 | `grpcx`、`identityclient`、`notifyclient`、`internalhttp`、`resilience`、`tlsutil`、`natsx`、`sharedapi` | gRPC/HTTP 客户端、重试、TLS、NATS 和跨服务契约适配 |
| 网关与事件 | `gatewayauth`、`wsticket`、`outbox`、`webhookx`、`audittrail`、`auditevents`、`oplog` | 网关鉴权、短期一次性 WebSocket 握手票据、可靠事件、Webhook、审计事件链路与跨服务操作日志批量落盘引擎 |
| 可观测性与任务 | `observability`、`jobbeat` | Trace/metrics 注入与任务心跳/执行观测 |
| 数据与业务适配 | `model`、`excel`、`exportproof`、`iploc`、`ipinfo`、`setting`、`mailer`、`idempotency` | 跨服务基础模型、导入导出、IP 定位、邮件、幂等和导出证明 |

## 消费者矩阵结论

- 高消费者基础包（`model`、`tenant`、`jwt`、`logger`、`response`、`pagination`）保留现有一级 package；改路径只会制造全仓 import churn，没有结构收益。
- 中低消费者包仍通过稳定的 shared package 被多个服务或 shared 适配器引用，暂不物理合并到其他类别。
- `health` 是七个基础设施服务 `/health*` 的单一真源；monitor 的 `/metrics` 仍留在本服务。
- `observability` 同时承载基础设施 `InitTracer`（结构化配置）和业务 `InitTracerFromEnv`（环境变量）；Gin 中间件已是单一实现。
- `redis` 是七个基础设施服务进程级 Redis 客户端 / pubsub 的单一真源。
- `database` 是七个基础设施服务进程级 GORM 客户端的单一真源；DSN / search_path 仍由各服务 config 计算。
- `captcha` 是登录验证码的单一真源；渲染用 auth 生产实现，存储由服务注入。
- `cache` 是七个基础设施服务会话 / 权限 / 验证码 / 字典 / OAuth2 缓存的单一真源。
- `ipinfo` 是 audit 登录日志与 monitor IP API 共用的 IP 地理位置客户端；monolith 保留单进程适配。
- `setting` 是七服务共用的 system_settings CRUD DAO；服务侧 alias 只为兼容既有 `internal/dao/system` import。
- `audittrail` 承载 GORM 审计插件与基础设施服务共用的 Gin 审计上下文注入中间件（`AttachGinAuditContext` / `ActorFromGinContext`）。
- `authdao` 是基础设施服务共用的用户查询、密码历史、TOTP/恢复码与认证 DAO 单一真源（`BaseUserDAO`、`AuthUserOps[U]`）；`auth`/`identity` 在嵌入基础上保留各自的租户扩展查询。
- `consoleauth` 是基础设施服务共用的 Web 控制台 Cookie 会话持久化/校验/吊销、角色码提取与会话响应构建单一真源（`ConsoleSessionService`、`BuildConsoleSession`、`ConsoleRoleCodes`）。
- `runtimeconfig` 承载 Redis 失效通知、泛型运行时配置缓存状态机以及基础设施服务共用的 `SecurityPolicy` 模型、缓存读取器与单实例持有器（`DefaultSecurityPolicyHolder`）；各服务仅保留本地配置默认值与扩展设置（如邮件、AI Provider）。
- `middleware` 除鉴权/限流基础组件外，同时承载基础设施服务共用的登录失败限流器与 IP 失败护盾（`LoginLimiter`、`LoginLimitConfig`、`RecordLoginFailureWithIPContext`、`IsIPBlockedContext`）。
- `wsticket` 是 `cc`、`im` 等实时通道共用的短期一次性 WebSocket 握手票据签发与单次消费验证器，避免浏览器把长期 Access JWT 暴露在 URL Query / 网关访问日志中。
- `errors`、`internalhttp`、`tlsutil` 存在 shared 包之间的传递消费者；不能用“直接 import 数量”判断为孤立包。
- 主仓特有的 `authz`、`tenantctx` 不同步到公开 micro 线；其消费者迁移由独立 authz 批次处理，本清单不覆盖并行实现。

## 维护约束

1. 新增跨服务能力先判断是否真的是 shared 基础设施，业务客户端不得借 shared 名义扩大公共面。
2. 新 package 必须在本清单登记职责和直接消费者；零消费者包必须说明保留理由。
3. 物理移动 shared package 需要同时更新主仓、micro、适用的 monolith 形态和全仓 import，并单独执行 `bash scripts/verify.sh all-go`。

消费者扫描基线：

```bash
python3 - <<'PY'
from pathlib import Path
import re
root = Path('microservices/services/shared/pkg')
files = list(Path('microservices').rglob('*.go'))
for package in sorted(p for p in root.iterdir() if p.is_dir()):
    prefix = 'github.com/go-admin-kit/services/shared/pkg/' + package.name
    consumers = [str(f) for f in files if not f.is_relative_to(package)
                 and re.search(r'"' + re.escape(prefix) + r'(?:/[^" ]*)?"', f.read_text(errors='ignore'))]
    print(package.name, len(consumers))
PY
```
