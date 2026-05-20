# Vue Router 循环引用错误修复

## 错误描述

```
Uncaught (in promise) TypeError: Converting circular structure to JSON
    --> starting at object with constructor 'ReactiveEffect'
    |     property 'deps' -> object with constructor 'Link'
    --- property 'sub' closes the circle
```

## 问题原因

在路由守卫 `permission.ts` 中，使用了 `{ ...to, replace: true }` 来传递路由对象。

`to` 是 Vue Router 的路由对象，它包含：
- 响应式属性（ReactiveEffect）
- 循环引用（deps -> Link -> sub）
- 内部状态和方法

当 Vue Router 尝试序列化这个对象时，会遇到循环引用，导致 JSON.stringify 失败。

## 修复方案

### 修复前（有问题）

```typescript
next(to.path === redirect ? { ...to, replace: true } : { path: redirect, query: to.query });
```

### 修复后（正确）

```typescript
if (to.path === redirect) {
  next({ path: to.path, query: to.query, params: to.params, replace: true });
} else {
  next({ path: redirect, query: to.query, replace: true });
}
```

## 关键点

1. **不要直接展开路由对象**：`{ ...to }` 会包含所有响应式属性和循环引用
2. **只传递必要的属性**：只传递 `path`、`query`、`params` 等可序列化的属性
3. **避免传递响应式对象**：不要传递包含响应式属性的对象

## 其他需要注意的地方

### ✅ 正确的方式

```typescript
// 只传递基本属性
next({ path: '/dashboard', query: { id: '1' }, replace: true });

// 从路由对象中提取需要的属性
next({
  path: to.path,
  query: to.query,
  params: to.params,
  hash: to.hash,
  replace: true
});
```

### ❌ 错误的方式

```typescript
// 不要直接展开路由对象
next({ ...to, replace: true });

// 不要传递整个路由对象
next(to);
```

## 相关文件

- 修复文件：`src/permission.ts`
- 路由配置：`src/router/index.ts`
- 路由工具：`src/utils/route/index.ts`

## 验证

修复后，应该不再出现循环引用错误。如果仍有问题，检查：

1. 是否在其他地方也使用了 `{ ...to }` 或 `{ ...from }`
2. 是否在路由配置中传递了响应式对象
3. 是否在路由守卫中尝试序列化路由对象

## 最佳实践

1. **路由跳转时**：只传递基本类型和可序列化的对象
2. **路由守卫中**：从路由对象中提取需要的属性，而不是传递整个对象
3. **避免序列化响应式对象**：不要在路由相关的代码中使用 JSON.stringify 序列化路由对象
