# Monolith 对账说明（2026-08-12 架构收口）

锚点从 `8de7c30a` 推进到 `027cc66068`。

## 已移植

- envsecret（`9c2e8c2`）

## 判定不适用（单进程 / 无服务间）

Consul、gRPC、grpcx、identityclient、natsx、outbox worker、auditevents、ConnPool、mTLS、schema-per-service、Swarm/chaos 109 实验、业务域服务拆分与 Phase 3–5 跨服务链。

## 可选后续

graceful 形态对齐（现有 signal+Shutdown 语义等价）。
