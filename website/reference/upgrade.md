# 版本升级

## 版本策略

- 遵循 [SemVer](https://semver.org/lang/zh-CN/)；**0.x 期间** API、数据库表结构与生成代码格式都可能变化，破坏性变更会在 [更新日志](/changelog) 与本页明示。
- 数据库迁移用 goose，**只前进不自动回退**——升级前备份是硬要求，回滚代码前先确认新迁移与旧代码兼容。
- 每个正式版都有 GitHub Release（notes 与更新日志同源）和 ghcr.io 版本化镜像。

## 通用升级步骤

**镜像部署（v0.2.0 起）**：

```bash
# 0) 备份（至少 pg_dump，见部署文档第 6 节）
cd /opt/gopherforge/microservices
export IMAGE_PREFIX=ghcr.io/superiorchuo/gopherforge/go-admin-kit
export IMAGE_TAG=v0.7.0            # 目标版本
docker compose pull && docker compose up -d --no-build
docker compose ps                   # migrate 自动跑新迁移后退出，等全部 healthy
```

回滚 = 把 `IMAGE_TAG` 切回上一版本重来一遍（迁移不回退，所以只适用于新迁移向后兼容的情形——0.x 的迁移我们尽量保持加列不删列，但以各版本注意事项为准）。

**源码构建**：`git pull` 到目标 tag → `make compose-up`。

## 0.6.0 → 0.7.0 注意事项

**1. 源码构建者：Go 工具链基线提升至 Go 1.27.0。** `go.work` 与各模块基线统一；shared 弹性调用真实采用了 Go 1.27 的 `Options.DoResult[T]` generic method，旧版本 Go 无法编译当前源码。使用官方镜像部署的用户不受影响。

**2. 前端路由 CSS 按需分包与组件拆分。** 认证、仪表盘、监控与日志样式随路由异步加载，首屏 CSS 大幅缩减；BPM 设计器与短信管理完成职责拆分，对外 API 与数据格式完全向后兼容。

**3. 公共 shared 底座收敛。** 数据库、缓存、Redis、验证码、健康检查、链路追踪与运行时配置订阅统一下沉到 `shared/pkg`，删除了各服务内的冗余副本。

## 0.5.0 → 0.6.0 注意事项

**1. 边缘证书生命周期 V2 与私钥加密。** ACME 账户私钥与证书私钥在数据库中启用 AES-256-GCM 封装加密；证书签发后由 `system-service` 写入 Traefik 动态配置目录，网关自动热重载，无需重启容器。

**2. 核心运行时与存储基线升级。** PostgreSQL 18、Redis 8、NATS 2.12、Traefik 3.7、Prometheus 3.13 与 Grafana 13。升级前请完成 `pg_dump` 备份。

**3. gRPC 契约与服务发现。** 服务间调用引入 Consul 服务发现与带连接池的 gRPC 客户端；BPM 审批发起支持幂等键。

## 0.4.0 → 0.5.0 注意事项

**1. 密码重置邮件链路。** 新增 `POST /password/forgot` 与 `POST /password/reset` 接口（迁移 `000060`）；启用该功能需在后台配置 SMTP 邮件服务。

**2. 租户存储配额。** 新增 `tenant_packages.storage_quota_mb` 字段（迁移 `000061`），默认 0 代表不限额度。

**3. 审计日志 CSV 导出。** 支持按筛选条件流式导出 UTF-8 BOM CSV 文件。

## 0.3.0 → 0.4.0 注意事项

**1. 统一任务中心。** 统一任务账本与服务心跳监控（迁移 `000050` 系列），离散任务逐次留痕。

**2. 通知模板体系。** 支持站内信、邮件与短信模板的集中管理与渠道路由。

## 0.2.0 → 0.3.0 注意事项

无破坏性变更，平滑升级；按影响排序：

**1. 安全依赖升级（建议尽快跟进）。** grpc 1.82.1（GHSA-hrxh-6v49-42gf）与 bpm 的 x/crypto 0.54 / x/net 0.56（2026-07-25 披露的 ssh/agent/knownhosts 与 html/idna 系列 HIGH CVE）。v0.2.0 镜像仍带漏洞版本依赖，这是切到 v0.3.0 最直接的理由。

**2. bpm 弱凭据判定收紧（唯一可能被感知的行为变化）。** 生产环境下 `dev-` 前缀形态的 token（如 `dev-xxx` 的 `BPM_INTERNAL_TOKEN`）将被视为占位符并 fail-closed 归零——internal 端点会返回 503。如果你确实在生产用这种形态的 token，升级前换成强随机值即可。

**3. 新增审计日志保留策略（opt-in，默认关闭）。** 设 `AUDIT_LOG_RETENTION_DAYS=N` 才生效，不设则行为与 0.2.0 完全一致；详见[审计日志](/modules/audit)。

**4. 官方镜像自本版起双架构**（`linux/amd64` + `linux/arm64`），arm64 部署不再需要本地构建。

## 0.1.0 → 0.2.0 注意事项

按影响从大到小：

**1. 生产弱凭据会被拒绝启动（行为变化，最容易踩）。** 0.2.0 给全部服务加了 `APP_ENV=production` 启动期校验：`JWT_SECRET` ≥32 位非占位、DB/Redis 密码非弱值、对象存储凭据（仅 s3/minio 模式）、OAuth client_secret 与 SMTP 密码（仅对应功能开启时）。**升级前先检查 `.env`**——0.1.0 带弱密码能跑，0.2.0 直接拒绝启动，报错信息会指明哪项不合格。这是护栏不是故障，把凭据换强即可。

**2. Compose 拆成双栈（首次切换有一步）。** 有状态服务移入 `docker-compose.infra.yml`（project 固定 `go-admin-kit-infra`），两栈经外部网络 `go-admin-kit-net` 互通。从 0.1.0 升级：先建网络（`make compose-up` 已内置该步骤），数据卷名不变、**数据零迁移**；此后操作 infra 栈必须显式 `-p go-admin-kit-infra`。

**3. 新增 bpm-service（审批流）。** 新服务 + 新表（迁移 000023/000025/000026/000027 自动跑）+ 网关新路由。内存预算加一个 Go 进程；不用审批流可以不配 `BPM_INTERNAL_TOKEN`（internal 端点会 503，其余功能不受影响）。

**4. 镜像名版本化（无感，但值得知道）。** compose 镜像名变为 `${IMAGE_PREFIX:-go-admin-kit}-<服务名>:${IMAGE_TAG:-latest}`——两个变量都不设时与 0.1.0 行为完全一致；设了即可用 ghcr 官方镜像部署与按版本回滚。

**5. 源码构建者：Go 工具链升至 1.26.5。** `go.work` 与各模块声明了 `toolchain go1.26.5`（消除 stdlib 可达漏洞）；`go` 语言版本指令未抬高，下游模块不被强迫升级。

**6. 新能力（选用）。** OAuth2/OIDC 授权服务端（含 PKCE、token 内省、JWKS）、运维管理面（任务心跳 / 服务健康总览 / Prometheus 告警闭环）、审批流加签与委派、全服务 `/metrics`、部门主管选人、Kubernetes 部署指南（`docs/deploy-k8s.md`）。功能清单详见[更新日志](/changelog)。

## rc 版本说明

`-rc.N` 预发布仅供尝鲜：CHANGELOG 可以没有对应段落（release notes 退化为提交摘要）、`latest` 镜像不更新。生产环境永远只跟正式版 tag。
