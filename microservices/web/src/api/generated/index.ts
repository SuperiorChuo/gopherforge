import type { components } from './schema'

/** OpenAPI 生成物里的 schema 名。新接口类型从这里取，不要再手写一份。 */
export type Schema<T extends keyof components['schemas']> = components['schemas'][T]
