# 单体版（规划中 / 未交付）

本目录预留给**独立的单体应用**产品线。

## 与微服务的关系

| 规则 | 说明 |
|------|------|
| 独立项目 | 与 `../microservices/` **业务零调用** |
| 不依赖 | 不引用 `services/*`、不强制 Traefik 多服务网关 |
| 前端 | 同样使用 React + Ant Design（阶段二从微服务前端再复制一份基线到 `web/`） |
| 后端 | 单进程 `server/`，内含全部业务域（阶段二从历史/`monolith` 分支恢复） |

## 当前状态

- 仅占位，**不可运行**
- 请先使用 [微服务版](../microservices/README.md)

## 计划结构（阶段二）

```text
monolith/
├── server/              # 完整单体 Go
├── web/                 # React Ant Design
├── docker-compose.yml   # postgres + redis + server + web
├── .env.example
└── README.md
```

在单体交付前，请勿在本目录开发业务功能。
