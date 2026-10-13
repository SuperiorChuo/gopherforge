# Prometheus 部署模式

- **Compose（默认）**：`prometheus.yml` 使用 Compose 容器名，并保留公开服务的 `service` 标签。
- **Swarm**：在 `microservices/.env` 设置 `PROMETHEUS_CONFIG_FILE=../platform/deploy/prometheus/prometheus.swarm.yml`，然后重新创建监控容器。配置按 `gak` stack 的 `tasks.gak_*-service` DNS 抓取每个副本；自定义 stack 名时同步修改 names 与 relabel 正则。
- 两种模式都要求 Prometheus 与被抓取服务共享网络；启用 `monitoring` profile 后抓取 PostgreSQL/Redis/node exporters。
- FreeSWITCH/control-api 是独立可选部署，默认不会访问外部媒体主机。部署后可在选定配置中追加 `fscc-control-api` job，`metrics_path: /metrics`，目标使用 Prometheus 容器实际可达的 control-api 地址。

Grafana 保留既有“服务概览”及其 UID `gak-services`（状态、QPS、错误率、P95、in-flight、Go 运行时及应用连接池）；新增“Platform Overview”展示 PostgreSQL、Redis、主机与告警指标。业务服务运行时指标使用 `go_admin_kit_go_*`，不是标准 Go collector 的 `go_*`。
