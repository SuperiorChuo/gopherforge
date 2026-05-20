// 统一导出所有 API，使用命名空间避免不同模块同名类型冲突。
export * as AuthAPI from './auth';
export * as PermissionRouteAPI from './permission';
export * as UserAPI from './system/user';
export * as RoleAPI from './system/role';
export * as SystemPermissionAPI from './system/permission';
export * as MenuAPI from './system/menu';
export * as FileAPI from './system/file';
export * as LoginLogAPI from './system/loginLog';
export * as OperationLogAPI from './system/operationLog';
export * as DictAPI from './system/dict';
export * as HealthAPI from './common/health';
