# 前端后端 API 对接文档

本文档说明前端项目与后端 Go 服务的 API 对接情况。

## 📋 对接完成情况

### ✅ 已完成对接的模块

1. **认证模块** (`src/api/auth.ts`)
   - ✅ 用户登录 (`POST /api/v1/login`)
   - ✅ 用户注册 (`POST /api/v1/register`)
   - ✅ 获取当前用户信息 (`GET /api/v1/user/me`)
   - ✅ 修改密码 (`PUT /api/v1/user/password`)
   - ✅ 刷新 Token (`POST /api/v1/refresh`)
   - ✅ 获取用户菜单 (`GET /api/v1/user/menus`)

2. **用户管理模块** (`src/api/system/user.ts`)
   - ✅ 获取用户列表 (`GET /api/v1/users`)
   - ✅ 获取用户详情 (`GET /api/v1/users/:id`)
   - ✅ 更新用户 (`PUT /api/v1/users/:id`)
   - ✅ 删除用户 (`DELETE /api/v1/users/:id`)
   - ✅ 更新用户状态 (`PUT /api/v1/users/:id/status`)
   - ✅ 分配角色 (`POST /api/v1/users/:id/roles`)

3. **角色管理模块** (`src/api/system/role.ts`)
   - ✅ 获取角色列表 (`GET /api/v1/roles`)
   - ✅ 获取所有角色 (`GET /api/v1/roles/all`)
   - ✅ 获取角色详情 (`GET /api/v1/roles/:id`)
   - ✅ 创建角色 (`POST /api/v1/roles`)
   - ✅ 更新角色 (`PUT /api/v1/roles/:id`)
   - ✅ 删除角色 (`DELETE /api/v1/roles/:id`)
   - ✅ 分配权限 (`POST /api/v1/roles/:id/permissions`)

4. **权限管理模块** (`src/api/system/permission.ts`)
   - ✅ 获取权限列表 (`GET /api/v1/permissions`)
   - ✅ 获取权限树 (`GET /api/v1/permissions/tree`)
   - ✅ 获取权限详情 (`GET /api/v1/permissions/:id`)
   - ✅ 创建权限 (`POST /api/v1/permissions`)
   - ✅ 更新权限 (`PUT /api/v1/permissions/:id`)
   - ✅ 删除权限 (`DELETE /api/v1/permissions/:id`)

5. **菜单管理模块** (`src/api/system/menu.ts`)
   - ✅ 获取菜单列表 (`GET /api/v1/menus`)
   - ✅ 获取菜单树 (`GET /api/v1/menus/tree`)
   - ✅ 获取菜单详情 (`GET /api/v1/menus/:id`)
   - ✅ 创建菜单 (`POST /api/v1/menus`)
   - ✅ 更新菜单 (`PUT /api/v1/menus/:id`)
   - ✅ 删除菜单 (`DELETE /api/v1/menus/:id`)

6. **文件管理模块** (`src/api/system/file.ts`)
   - ✅ 上传单个文件 (`POST /api/v1/files/upload`)
   - ✅ 批量上传文件 (`POST /api/v1/files/upload/multiple`)
   - ✅ 获取文件列表 (`GET /api/v1/files`)
   - ✅ 获取我的文件 (`GET /api/v1/files/my`)
   - ✅ 获取文件统计 (`GET /api/v1/files/stats`)
   - ✅ 获取文件详情 (`GET /api/v1/files/:id`)
   - ✅ 下载文件 (`GET /api/v1/files/:id/download`)
   - ✅ 预览文件 (`GET /api/v1/files/:id/preview`)
   - ✅ 删除文件 (`DELETE /api/v1/files/:id`)
   - ✅ 批量删除文件 (`DELETE /api/v1/files/batch`)

7. **登录日志模块** (`src/api/system/loginLog.ts`)
   - ✅ 获取登录日志列表 (`GET /api/v1/login-logs`)
   - ✅ 获取我的登录日志 (`GET /api/v1/login-logs/my`)
   - ✅ 获取登录统计 (`GET /api/v1/login-logs/stats`)
   - ✅ 获取最后登录信息 (`GET /api/v1/login-logs/last`)
   - ✅ 获取用户登录历史 (`GET /api/v1/login-logs/user/:user_id`)
   - ✅ 清理登录日志 (`DELETE /api/v1/login-logs/clear`)

8. **操作日志模块** (`src/api/system/operationLog.ts`)
   - ✅ 获取操作日志列表 (`GET /api/v1/operation-logs`)
   - ✅ 获取操作日志统计 (`GET /api/v1/operation-logs/stats`)
   - ✅ 获取操作日志详情 (`GET /api/v1/operation-logs/:id`)
   - ✅ 导出操作日志 (`GET /api/v1/operation-logs/export`)
   - ✅ 清理操作日志 (`DELETE /api/v1/operation-logs/clear`)

9. **数据字典模块** (`src/api/system/dict.ts`)
   - ✅ 字典类型管理（CRUD）
   - ✅ 字典项管理（CRUD）
   - ✅ 根据编码获取字典数据 (`GET /api/v1/dicts/:code`)
   - ✅ 批量获取字典数据 (`GET /api/v1/dicts?codes=...`)
   - ✅ 获取所有字典数据 (`GET /api/v1/dicts/all`)

## 🔧 配置说明

### 环境变量配置

开发环境配置文件：`.env.development`

```env
VITE_BASE_URL = /
VITE_IS_REQUEST_PROXY = true
VITE_API_URL = http://localhost:8081
VITE_API_URL_PREFIX = /api/v1
```

### Vite 代理配置

在 `vite.config.ts` 中配置了代理，将 `/api/v1` 请求代理到后端服务器：

```typescript
proxy: {
  [VITE_API_URL_PREFIX]: {
    target: 'http://localhost:8081',
    changeOrigin: true,
    rewrite: (path) => path.replace(new RegExp(`^${VITE_API_URL_PREFIX}`), VITE_API_URL_PREFIX),
  },
}
```

## 📝 使用示例

### 登录

```typescript
import { login } from '@/api/auth';

const handleLogin = async () => {
  try {
    const response = await login({
      username: 'admin',
      password: 'admin123',
    });
    // 登录成功，token 已自动保存到 store
    return response;
  } catch (error) {
    console.error('登录失败', error);
  }
};
```

### 获取用户信息

```typescript
import { getCurrentUser } from '@/api/auth';
import { useUserStore } from '@/store';

const userStore = useUserStore();
await userStore.getUserInfo();
```

### 获取用户列表

```typescript
import { getUserList } from '@/api/system/user';

const fetchUsers = async () => {
  const response = await getUserList({
    page: 1,
    page_size: 10,
    keyword: 'admin',
  });
  return response;
};
```

## 🔐 认证机制

### Token 存储

- Token 存储在 Pinia store (`src/store/modules/user.ts`)
- 使用 `pinia-plugin-persistedstate` 持久化存储
- Token 自动添加到请求头：`Authorization: Bearer {token}`

### 请求拦截器

- 自动添加 Token 到请求头
- 处理 401 未授权错误，自动跳转到登录页
- 统一处理后端响应格式：`{ code: 200, message: "success", data: {...} }`

### 响应拦截器

- 统一处理后端响应格式
- 自动处理错误信息
- 401 错误自动登出并跳转登录页

## 🎯 菜单路由对接

后端返回的菜单格式会自动转换为前端路由格式：

**后端格式：**
```json
{
  "id": 1,
  "name": "dashboard",
  "title": "仪表盘",
  "icon": "dashboard",
  "path": "/dashboard",
  "component": "dashboard/index",
  "children": [...]
}
```

**前端格式：**
```typescript
{
  path: "/dashboard",
  name: "dashboard",
  component: "dashboard/index",
  meta: {
    title: "仪表盘",
    icon: "dashboard"
  },
  children: [...]
}
```

转换逻辑在 `src/api/permission.ts` 中的 `transformMenuToRoute` 函数。

## 📦 API 文件结构

```
src/api/
├── auth.ts              # 认证相关 API
├── permission.ts        # 权限/菜单 API
├── index.ts            # 统一导出
└── system/
    ├── user.ts         # 用户管理
    ├── role.ts         # 角色管理
    ├── permission.ts   # 权限管理
    ├── menu.ts         # 菜单管理
    ├── file.ts         # 文件管理
    ├── loginLog.ts     # 登录日志
    ├── operationLog.ts # 操作日志
    └── dict.ts         # 数据字典
```

## 🚀 启动说明

1. **启动后端服务**
   ```bash
   cd server
   go run cmd/main.go
   # 后端运行在 http://localhost:8081
   ```

2. **启动前端服务**
   ```bash
   cd tdesign-vue-go
   npm install
   npm run dev
   # 前端运行在 http://localhost:3002
   ```

3. **访问应用**
   - 前端地址：http://localhost:3002
   - 默认登录账号：`admin` / `admin`（根据后端数据库配置）

## ⚠️ 注意事项

1. **CORS 配置**：确保后端已配置允许前端域名的跨域请求
2. **Token 过期**：Token 过期会自动跳转到登录页，需要重新登录
3. **菜单权限**：菜单会根据用户权限动态加载，无权限的菜单不会显示
4. **错误处理**：所有 API 调用都应使用 try-catch 处理错误

## 📚 相关文档

- 后端 API 文档：`server/FEATURES_V2.md`
- 后端路由配置：`server/internal/api/routes.go`
- 前端路由配置：`src/router/index.ts`
