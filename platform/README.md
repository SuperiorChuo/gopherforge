# 公共底座（platform）

跨产品线可复用、但不绑定某一部署形态的资源。

| 路径 | 说明 |
|------|------|
| `deploy/` | Prometheus / Grafana / OpenTelemetry 等配置模板 |

微服务编排通过相对路径引用 `../platform/deploy/`。  
单体交付后可按需复用同一套监控模板，**不**在此放置业务服务代码。
