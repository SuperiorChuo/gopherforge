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
export IMAGE_TAG=v0.2.0            # 目标版本
docker compose pull && docker compose up -d --no-build
docker compose ps                   # migrate 自动跑新迁移后退出，等全部 healthy
```

回滚 = 把 `IMAGE_TAG` 切回上一版本重来一遍（迁移不回退，所以只适用于新迁移向后兼容的情形——0.x 的迁移我们尽量保持加列不删列，但以各版本注意事项为准）。

**源码构建**：`git pull` 到目标 tag → `make compose-up`。

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
