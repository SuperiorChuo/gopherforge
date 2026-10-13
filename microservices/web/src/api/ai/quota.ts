import request from '@/utils/request';

// 配额接口类型
export interface AIQuota {
  id: number;
  scope: string;
  scope_id?: number;
  daily_token_limit?: number;
  daily_cost_limit?: number;
  daily_request_limit?: number;
  monthly_token_limit?: number;
  monthly_cost_limit?: number;
  monthly_request_limit?: number;
  reset_day_of_month: number;
  enabled: number;
  created_at: string;
  updated_at: string;
}

export interface AIQuotaListParams {
  scope?: string;
}

// 查询配额列表
export const getAIQuotaList = (params?: AIQuotaListParams) => {
  return request.get<unknown, AIQuota[]>('/api/v1/ai/quotas', { params });
};

// 创建或更新配额
export const upsertAIQuota = (data: Partial<AIQuota>) => {
  return request.post<unknown, AIQuota>('/api/v1/ai/quotas', data);
};

// 删除配额
export const deleteAIQuota = (id: number) => {
  return request.delete<unknown, void>(`/api/v1/ai/quotas/${id}`);
};
