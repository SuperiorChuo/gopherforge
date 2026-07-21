import request from '@/utils/request'
import type { PageRequest, PageResponse } from '@/types'

/** 岗位实体（system:post） */
export interface SystemPost {
  id: number
  tenant_id?: number
  code: string
  name: string
  sort: number
  status: number
  remark?: string
  created_at?: string
}

type PostListParams = PageRequest & { keyword?: string; status?: number }
type PostCreateData = {
  code: string
  name: string
  sort?: number
  status?: number
  remark?: string
}
type PostUpdateData = Partial<PostCreateData>

export const getPostList = (params: PostListParams) =>
  request.get<unknown, PageResponse<SystemPost>>('/api/v1/posts', { params })

export const getAllPosts = (status?: number) =>
  request.get<unknown, SystemPost[]>('/api/v1/posts/all', {
    params: status === undefined ? undefined : { status },
  })

export const getPost = (id: number) =>
  request.get<unknown, SystemPost>(`/api/v1/posts/${id}`)

export const createPost = (data: PostCreateData) =>
  request.post<unknown, SystemPost>('/api/v1/posts', data)

export const updatePost = (id: number, data: PostUpdateData) =>
  request.put<unknown, SystemPost>(`/api/v1/posts/${id}`, data)

export const deletePost = (id: number) =>
  request.delete<unknown, void>(`/api/v1/posts/${id}`)
