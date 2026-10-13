import request from '@/utils/request'

// AI 模型接口类型定义
export interface AIModel {
  id: number
  name: string
  display_name: string
  provider: string
  api_key?: string
  base_url?: string
  model_config?: Record<string, unknown>
  capabilities: string[]
  status: number
  cost_per_1k_input: number
  cost_per_1k_output: number
  cost_per_image?: number
  daily_limit?: number
  description?: string
  sort: number
  created_at: string
  updated_at: string
}

export interface AIModelListParams {
  page?: number
  page_size?: number
  provider?: string
  status?: number
}

export interface AIModelListResponse {
  list: AIModel[]
  total: number
}

// 查询模型列表
export const getAIModelList = (params: AIModelListParams) =>
  request.get<unknown, AIModelListResponse>('/api/v1/ai/models', { params })

// 查询启用的模型
export const getEnabledAIModels = () =>
  request.get<unknown, AIModel[]>('/api/v1/ai/models/enabled')

// 查询模型详情
export const getAIModel = (id: number) =>
  request.get<unknown, AIModel>(`/api/v1/ai/models/${id}`)

// 创建模型
export const createAIModel = (data: Partial<AIModel>) =>
  request.post<unknown, AIModel>('/api/v1/ai/models', data)

// 更新模型
export const updateAIModel = (id: number, data: Partial<AIModel>) =>
  request.put<unknown, AIModel>(`/api/v1/ai/models/${id}`, data)

// 删除模型
export const deleteAIModel = (id: number) =>
  request.delete<unknown, void>(`/api/v1/ai/models/${id}`)
