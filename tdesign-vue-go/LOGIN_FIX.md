# 登录问题修复说明

## 问题描述

登录时出现错误：`{"code":401,"message":"invalid username or password"}`

## 已修复的问题

### 1. 默认密码不匹配 ✅

**问题**：前端默认密码是 `admin`，但数据库默认密码是 `admin123`

**修复**：已更新前端登录组件的默认密码为 `admin123`

文件：`src/pages/login/components/Login.vue`
```typescript
const INITIAL_DATA = {
  account: 'admin',
  password: 'admin123', // 已更新
  // ...
};
```

### 2. 错误处理优化 ✅

**问题**：401 错误处理逻辑不完善

**修复**：优化了响应拦截器的错误处理，确保：
- 登录失败时正确显示错误信息
- 不会在登录页面清除 token（避免循环跳转）
- 错误信息能正确传递给用户

## 默认登录凭据

- **用户名**：`admin`
- **密码**：`admin123`

## 验证步骤

1. **确认数据库已初始化**
   ```bash
   mysql -u root -p < server/docs/go_admin_kit.sql
   ```

2. **验证用户存在**
   ```sql
   SELECT id, username, email, status FROM users WHERE username = 'admin';
   ```

3. **测试登录**
   - 打开前端页面：http://localhost:3002
   - 使用默认账号登录：`admin` / `admin123`
   - 如果仍然失败，检查后端日志

## 如果仍然无法登录

### 检查清单

- [ ] 数据库是否已初始化？
- [ ] 用户是否存在且状态为启用（status = 1）？
- [ ] 后端服务是否正常运行？
- [ ] 前端环境变量配置是否正确？
- [ ] 网络请求是否到达后端？（查看浏览器 Network 面板）

### 调试方法

1. **查看浏览器控制台**
   - 打开开发者工具（F12）
   - 查看 Console 和 Network 面板
   - 检查请求 URL、请求体、响应内容

2. **查看后端日志**
   - 检查后端控制台输出
   - 查看是否有错误信息

3. **直接测试 API**
   ```bash
   curl -X POST http://localhost:8081/api/v1/login \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"admin123"}'
   ```

4. **检查数据库密码哈希**
   ```sql
   SELECT password FROM users WHERE username = 'admin';
   ```
   默认密码 `admin123` 的哈希值应该是：
   `$2a$10$N.zmdr9k7uOCQb376NoUnuTJ8iAt6Z5EHsM8lE9lBpwTTyU3VxqJe`

## 创建新用户

如果默认用户不存在，可以通过注册接口创建：

```bash
curl -X POST http://localhost:8081/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "Admin123",
    "email": "admin@example.com"
  }'
```

**注意**：密码必须符合以下要求：
- 至少 8 个字符
- 最多 32 个字符
- 包含大写字母、小写字母和数字

## 相关文件

- 登录组件：`src/pages/login/components/Login.vue`
- 用户 Store：`src/store/modules/user.ts`
- 认证 API：`src/api/auth.ts`
- 请求拦截器：`src/utils/request/index.ts`
- 数据库初始化：`server/docs/go_admin_kit.sql`
- 故障排查文档：`server/docs/TROUBLESHOOTING.md`
