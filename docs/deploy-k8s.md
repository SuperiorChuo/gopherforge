# Kubernetes 部署指南（k3s 起步 → 多节点 → 托管集群）

面向把本脚手架部署到 Kubernetes 的运维/自部署用户。
单机 Docker Compose 生产部署见 [`deployment.md`](deployment.md)——**如果你只有 1~3 台机器且没有 K8s 经验，优先走那条路线（或 Docker Swarm）**，本文是给确定要上 K8s（自建 k3s 或云托管 EKS/ACK/TKE）的人准备的迁移地图。

---

## 1. 为什么这套脚手架可以平移到 K8s

代码与部署形态已经满足 K8s 的全部前置，**不需要改任何业务代码**：

| 前置条件 | 本项目现状 |
|----------|-----------|
| 应用服务无状态 | 会话在 Redis、JWT 无本地状态；上传文件可走 MinIO/S3 对象存储（`/uploads` 由 file-service 动态回源） |
| 配置外置 | 全部走环境变量（`.env`），运行时参数在 DB `system_settings` 热生效 |
| 有状态服务独立 | PG / Redis / NATS / MinIO 已拆为独立 infra 栈（`docker-compose.infra.yml`），应用栈不含数据 |
| 健康检查 | 每个服务自带 `/health/ready` 端点（compose healthcheck 可直译 probes） |
| 迁移与启动解耦 | goose 迁移是独立一次性 job（monitor 镜像 `./migrate`），自带有界重试 |
| 网关声明式路由 | Traefik 路由规则全部是声明式的，可直译 IngressRoute |

## 2. 架构映射表（Compose → K8s）

| Compose 里的东西 | K8s 对应物 |
|------------------|-----------|
| 应用服务（monitor/auth/identity/system/audit/file/bpm + frontend） | `Deployment` + `Service`（每服务一对） |
| `healthcheck`（wget /health/ready） | `readinessProbe` + `livenessProbe`（httpGet） |
| `migrate` 一次性 job | `Job`（每次发版前 apply 一次） |
| `.env` 密钥（JWT_SECRET、DB 密码等） | `Secret` |
| `.env` 非密钥配置 | `ConfigMap`（或直接写 Deployment env） |
| infra 栈 PG / Redis / NATS / MinIO | 开发：`StatefulSet`+PVC；**生产强烈建议托管**（RDS / 云 Redis / 云对象存储），详见 §5 |
| Traefik 网关 + Docker labels | k3s 内置 Traefik → `IngressRoute` + `Middleware` CRD（§8） |
| ForwardAuth 中间件 | Traefik `Middleware` 的 `forwardAuth`，指向 auth-service（§8） |
| compose 网络 / 容器名 DNS | CNI 自带跨节点网络；**K8s Service 命名沿用现有容器名**（§3，关键技巧） |
| `restart: unless-stopped` | Deployment 自带；另获得副本数、滚动更新、节点漂移 |
| ip2region.xdb 只读挂载 | initContainer 下载到 `emptyDir`（§10） |

## 3. 关键约定：Service 命名沿用容器名，环境变量零改动

所有服务互相寻址用的是 compose 容器名（`DB_HOST=go-admin-kit-postgres`、`NATS_URL=nats://go-admin-kit-nats:4222`…）。**把 K8s Service 的 `metadata.name` 取成同样的名字**，集群 DNS 就能原样解析，所有环境变量默认值直接复用：

```yaml
apiVersion: v1
kind: Service
metadata:
  name: go-admin-kit-auth        # ← 与 compose 容器名一致
  namespace: go-admin-kit
spec:
  selector: { app: auth-service }
  ports: [{ port: 8082, targetPort: 8082 }]
```

每个服务照此办理：`go-admin-kit-postgres`(5432)、`go-admin-kit-redis`(6379)、`go-admin-kit-nats`(4222)、`go-admin-kit-minio`(9000)、`go-admin-kit-monitor`(8081)、`go-admin-kit-auth`(8082)、`go-admin-kit-identity`(8083)、`go-admin-kit-system`(8084)、`go-admin-kit-audit`(8085)、`go-admin-kit-file`(8086)、`go-admin-kit-bpm`(8096)、`go-admin-kit-frontend`(80)。

## 4. k3s 快速开始（单机起步）

```bash
# 装 k3s（自带 containerd、Traefik Ingress、local-path 存储类）
curl -sfL https://get.k3s.io | sh -
kubectl get nodes                      # Ready 即可

# 加工作节点（未来扩容）
# 主节点拿 token：cat /var/lib/rancher/k3s/server/node-token
# 新机器：curl -sfL https://get.k3s.io | K3S_URL=https://<主节点IP>:6443 K3S_TOKEN=<token> sh -
```

k3s 内置 Traefik（CRD `apiVersion: traefik.io/v1alpha1`，旧版本用 `traefik.containo.us/v1alpha1`，以 `kubectl get crd | grep traefik` 为准），本项目网关规则可直译，不必另装 Ingress 控制器。

```bash
kubectl create namespace go-admin-kit
```

## 5. 有状态服务

**生产原则：数据库不进集群。** 优先用云 RDS PostgreSQL（注意要带 pgvector 扩展）、云 Redis、云对象存储（S3 兼容，file-service 原生支持 `UPLOAD_STORAGE_TYPE=s3`）；NATS 用官方 Helm chart 起 3 副本 JetStream 集群。此时应用侧只需改 4 个 env（DB_HOST/REDIS_HOST/NATS_URL/UPLOAD_S3_*）。

**开发/内网自建**（k3s 单机，等价于 infra 栈）：

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata: { name: go-admin-kit-postgres, namespace: go-admin-kit }
spec:
  serviceName: go-admin-kit-postgres
  replicas: 1
  selector: { matchLabels: { app: postgres } }
  template:
    metadata: { labels: { app: postgres } }
    spec:
      containers:
      - name: postgres
        image: pgvector/pgvector:pg16
        ports: [{ containerPort: 5432 }]
        envFrom: [{ secretRef: { name: go-admin-kit-secrets } }]
        volumeMounts: [{ name: data, mountPath: /var/lib/postgresql/data }]
        readinessProbe:
          exec: { command: ["sh", "-c", "pg_isready -U $POSTGRES_USER -d $POSTGRES_DB"] }
          periodSeconds: 10
  volumeClaimTemplates:
  - metadata: { name: data }
    spec: { accessModes: [ReadWriteOnce], resources: { requests: { storage: 20Gi } } }
```

Redis / NATS / MinIO 同理（Redis 记得 `--requirepass`，NATS 带 `--jetstream --store_dir` + PVC；MinIO 建议直接用 MinIO Operator 或云对象存储）。

**存量数据迁移**：`pg_dumpall | psql` 灌入新库；MinIO 用 `mc mirror` 双向同步后切流。

## 6. 密钥、配置与迁移 Job

```bash
kubectl -n go-admin-kit create secret generic go-admin-kit-secrets \
  --from-literal=POSTGRES_USER=go_admin_kit \
  --from-literal=POSTGRES_PASSWORD='<强密码>' \
  --from-literal=POSTGRES_DB=go_admin_kit \
  --from-literal=REDIS_PASSWORD='<强密码>' \
  --from-literal=JWT_SECRET='<≥32位>' \
  --from-literal=MINIO_ROOT_USER=... --from-literal=MINIO_ROOT_PASSWORD=...
```

迁移 Job（对应 compose 的 `migrate` 服务；镜像自带有界重试，PG 未就绪会等）：

```yaml
apiVersion: batch/v1
kind: Job
metadata: { name: go-admin-kit-migrate, namespace: go-admin-kit }
spec:
  backoffLimit: 3
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: migrate
        image: <registry>/go-admin-kit-monitor-service:<tag>
        command: ["sh", "-c", "n=0; until ./migrate -config ./configs/config.yaml -dir ./migrations up; do n=$((n+1)); [ $n -ge 30 ] && exit 1; sleep 2; done"]
        env:
        - { name: DB_HOST, value: go-admin-kit-postgres }
        - { name: DB_PORT, value: "5432" }
        - { name: DB_USER, valueFrom: { secretKeyRef: { name: go-admin-kit-secrets, key: POSTGRES_USER } } }
        - { name: DB_PASSWORD, valueFrom: { secretKeyRef: { name: go-admin-kit-secrets, key: POSTGRES_PASSWORD } } }
        - { name: DB_NAME, valueFrom: { secretKeyRef: { name: go-admin-kit-secrets, key: POSTGRES_DB } } }
```

> K8s 没有 compose 的 `depends_on: service_completed_successfully`。做法：发版脚本先 `kubectl apply` Job 并 `kubectl wait --for=condition=complete job/go-admin-kit-migrate`，再滚动应用服务。服务本身连不上库会 CrashLoop 重试，最终一致，不会坏数据。

## 7. 应用服务模板（以 auth-service 为例，其余同构）

```yaml
apiVersion: apps/v1
kind: Deployment
metadata: { name: auth-service, namespace: go-admin-kit }
spec:
  replicas: 2                                  # K8s 下第一个红利：副本
  selector: { matchLabels: { app: auth-service } }
  template:
    metadata: { labels: { app: auth-service } }
    spec:
      containers:
      - name: auth
        image: <registry>/go-admin-kit-auth-service:<tag>
        ports: [{ containerPort: 8082 }]
        env:
        - { name: APP_ENV, value: production }
        - { name: APP_PORT, value: "8082" }
        - { name: DB_HOST, value: go-admin-kit-postgres }
        - { name: REDIS_HOST, value: go-admin-kit-redis }
        - { name: NATS_URL, value: "nats://go-admin-kit-nats:4222" }
        - { name: TRUSTED_PROXIES, value: "10.42.0.0/16" }   # ← k3s Pod CIDR，替换 compose 的 172.28.0.0/16
        - { name: DB_USER, valueFrom: { secretKeyRef: { name: go-admin-kit-secrets, key: POSTGRES_USER } } }
        - { name: DB_PASSWORD, valueFrom: { secretKeyRef: { name: go-admin-kit-secrets, key: POSTGRES_PASSWORD } } }
        - { name: DB_NAME, valueFrom: { secretKeyRef: { name: go-admin-kit-secrets, key: POSTGRES_DB } } }
        - { name: REDIS_PASSWORD, valueFrom: { secretKeyRef: { name: go-admin-kit-secrets, key: REDIS_PASSWORD } } }
        - { name: JWT_SECRET, valueFrom: { secretKeyRef: { name: go-admin-kit-secrets, key: JWT_SECRET } } }
        readinessProbe:
          httpGet: { path: /api/v1/health/ready, port: 8082 }
          periodSeconds: 10
        livenessProbe:
          httpGet: { path: /api/v1/health/live, port: 8082 }
          periodSeconds: 10
          initialDelaySeconds: 20
        resources:
          requests: { cpu: 50m, memory: 64Mi }
          limits: { memory: 256Mi }
```

各服务差异只有三处：**端口**（§3 表）、**健康路径前缀**（bpm 是 `/api/v1/bpm/health/ready`——照抄 compose healthcheck 里的 URL）、**专属 env**（照抄 `docker-compose.yml` 对应服务的 environment 段）。`TRUSTED_PROXIES` 全部换成集群 Pod CIDR。

## 8. 网关：Traefik IngressRoute + ForwardAuth 平移

compose 的 Docker labels 直译成 CRD。**ForwardAuth 中间件**（对应 `auth-verify`）：

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata: { name: auth-verify, namespace: go-admin-kit }
spec:
  forwardAuth:
    address: http://go-admin-kit-auth:8082/internal/verify
    authResponseHeaders: [X-Auth-User-ID, X-Auth-Username, X-Auth-Tenant-ID, X-Auth-Platform-Admin]
```

路由规则原样照抄 labels 里的 rule 字符串（`Path()`/`PathPrefix()` 语法相同，**PathPrefix 是裸前缀匹配的坑同样存在**，见 compose 注释）。示例——auth 公开路由 + identity 受保护路由 + bpm（自带 Bearer 鉴权，不挂 ForwardAuth）：

```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata: { name: gak-routes, namespace: go-admin-kit }
spec:
  entryPoints: [web]
  routes:
  - match: "Path(`/api/v1/login`) || PathPrefix(`/api/v1/captcha/`) || Path(`/api/v1/captcha`) || PathPrefix(`/api/v1/oauth/`) || PathPrefix(`/api/v1/oauth2/`) || PathPrefix(`/api/v1/auth/`) || Path(`/api/v1/user/me`)"
    priority: 100
    services: [{ name: go-admin-kit-auth, port: 8082 }]
  - match: "Path(`/api/v1/users`) || PathPrefix(`/api/v1/users/`) || Path(`/api/v1/roles`) || PathPrefix(`/api/v1/roles/`)"
    priority: 100
    middlewares: [{ name: auth-verify }]
    services: [{ name: go-admin-kit-identity, port: 8083 }]
  - match: "Path(`/api/v1/bpm`) || PathPrefix(`/api/v1/bpm/`)"
    priority: 100
    services: [{ name: go-admin-kit-bpm, port: 8096 }]
  # …其余服务照 docker-compose.yml 各 labels 段逐条翻译…
  - match: "PathPrefix(`/uploads/`)"
    priority: 100
    services: [{ name: go-admin-kit-file, port: 8086 }]
  - match: "PathPrefix(`/`)"                   # 前端 SPA 兜底
    priority: 1
    services: [{ name: go-admin-kit-frontend, port: 80 }]
```

TLS：给 IngressRoute 挂 `websecure` entryPoint + cert-manager（Let's Encrypt），或沿用 deployment.md 的「前置 Nginx 终止 TLS」模式。

## 9. 镜像仓库与构建

K8s 拉镜像必须有 registry（compose 的本机 build 模式不再适用）：

- 内网：起个 registry（`registry:2` 或 Harbor），构建推送：
  ```bash
  cd microservices
  docker build -t <registry>/go-admin-kit-auth-service:v1 -f services/auth/Dockerfile services/
  docker push <registry>/go-admin-kit-auth-service:v1
  ```
  （各服务 build context/dockerfile 参数照抄 `docker-compose.yml` 的 build 段；k3s 用自签/HTTP registry 需配 `/etc/rancher/k3s/registries.yaml`）
- 云上：用云厂商镜像仓库 + CI 流水线构建推送。

## 10. ip2region 离线库挂载

system/audit 只读挂 `/app/data/ip2region.xdb`（缺失时优雅降级）。K8s 下用 initContainer 跑 `scripts/download-ip2region.sh` 下到 `emptyDir`，或直接打进镜像。**权限必须 644**（服务进程是非 root 用户）。

## 11. 观测

Prometheus + Grafana 用 `kube-prometheus-stack` Helm chart 替代 compose 的 monitoring profile；monitor-service 的 `/metrics` 加 `ServiceMonitor` 抓取。Jaeger/OTel 同理用各自 Helm chart，`TRACING_OTLP_ENDPOINT` 指向集群内 collector Service。

## 12. 迁移实操顺序（从 Compose 部署切换）

1. 起 registry，全部服务镜像构建推送
2. k3s 装好，`kubectl apply` 顺序：Namespace → Secret → 有状态（或托管连通性验证）→ migrate Job（wait complete）→ 各 Deployment/Service → Middleware/IngressRoute
3. 数据迁移：PG dump 灌入 + MinIO `mc mirror`
4. 流量切换：入口反代的 upstream 从 Traefik 改指 k3s Ingress；`npm run smoke:api` 全绿后下线 compose 栈
5. 回滚预案：compose 栈保留不删，切回 upstream 即回退（注意切换窗口内的新数据要反向同步）

## 13. 与 Swarm 路线的关系

两条路线共享全部前置（本仓已具备：双栈拆分、服务无状态化、对象存储、声明式网关）。Swarm 是「compose 平滑升级」，K8s 是「生态与托管」——**1~3 台机器用 Swarm/compose 够用；要上云托管、要 HPA/operator 生态、要私有化交付时再走本文**。两者不必都做。
