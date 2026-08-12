import { createApi } from '@reduxjs/toolkit/query/react'
import type { BaseQueryFn } from '@reduxjs/toolkit/query'
import type { AxiosRequestConfig } from 'axios'
import instance from '@/utils/request'

interface AxiosBaseQueryArgs {
  url: string
  method: AxiosRequestConfig['method']
  data?: AxiosRequestConfig['data']
  params?: AxiosRequestConfig['params']
  headers?: AxiosRequestConfig['headers']
}

const axiosBaseQuery = (): BaseQueryFn<AxiosBaseQueryArgs, unknown, unknown> => async (args) => {
  try {
    const result = await instance({
      url: args.url,
      method: args.method,
      data: args.data,
      params: args.params,
      headers: args.headers,
    })
    return { data: result }
  } catch (err) {
    const error = err as { message?: string; response?: { status: number } }
    return {
      error: {
        status: error.response?.status ?? 500,
        data: error.message ?? '请求失败',
      },
    }
  }
}

export const apiSlice = createApi({
  reducerPath: 'api',
  baseQuery: axiosBaseQuery(),
  tagTypes: ['User', 'Role', 'Menu', 'Permission', 'Dict', 'Monitor', 'AuditLog', 'File'],
  endpoints: () => ({}),
})
