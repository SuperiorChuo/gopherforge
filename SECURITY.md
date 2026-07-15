# 安全策略

Go Admin Kit 是后台管理脚手架，默认配置用于本地开发。生产或公网环境部署前，必须替换默认密钥、默认密码和 CORS 配置。

## 支持范围

当前仅维护 `main` 分支。请优先基于最新 `main` 复现和提交安全问题。

## 报告安全问题

请不要在公开 issue 中披露完整攻击步骤、可利用 payload、真实密钥或生产数据。

推荐方式：

1. 优先使用 GitHub Security Advisory 私密上报。
2. 如果当前仓库未开启私密上报，请创建一个不包含利用细节的 issue，说明问题类型和影响范围，并等待维护者沟通后再提供复现细节。
3. 如问题已经被公开利用，请在报告中标注“疑似已被利用”，方便优先处理。

## 生产部署最低要求

上线前至少完成这些配置：

```bash
APP_ENV=production
JWT_SECRET=至少32位随机字符串
MYSQL_ROOT_PASSWORD=强密码
REDIS_PASSWORD=强密码
CORS_ALLOW_ORIGINS=https://你的前端域名
CORS_ALLOW_CREDENTIALS=true
SECURITY_HSTS_ENABLED=true
DEFAULT_ADMIN_FORCE_CHANGE_PASSWORD=true
```

同时请确认：

- 默认管理员密码已修改。
- MySQL、Redis、MinIO、Grafana 等服务不暴露弱密码。
- 上传目录或对象存储 bucket 已隔离。
- 反向代理已配置 HTTPS、HSTS 和可信代理地址。
- 生产日志不输出密码、token、secret 或用户隐私字段。

更多安全能力说明见：

- `docs/SECURITY.md`
- 上线前请替换默认密钥与管理员密码（见 README「安全提示」）
