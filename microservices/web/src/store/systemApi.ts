import { apiSlice } from './api'
import type { PageRequest, PageResponse, SystemUser, SystemRole, MenuItem, Permission } from '@/types'

type Role = SystemRole

type UserListParams = PageRequest & { keyword?: string; status?: number }
type UserCreateData = Omit<SystemUser, 'id' | 'created_at'> & { password?: string; post_ids?: number[] }
type UserUpdateData = Partial<UserCreateData>

interface RoleListParams extends PageRequest { keyword?: string }

export const systemApi = apiSlice.injectEndpoints({
  endpoints: (builder) => ({
    getUserList: builder.query<PageResponse<SystemUser>, UserListParams>({
      query: (params) => ({ url: '/api/v1/users', method: 'GET', params }),
      providesTags: (result) =>
        result
          ? [
              ...result.list.map((u) => ({ type: 'User' as const, id: u.id })),
              { type: 'User' as const, id: 'LIST' },
            ]
          : [{ type: 'User' as const, id: 'LIST' }],
    }),
    createUser: builder.mutation<SystemUser, UserCreateData>({
      query: (data) => ({ url: '/api/v1/users', method: 'POST', data }),
      invalidatesTags: [{ type: 'User', id: 'LIST' }],
    }),
    updateUser: builder.mutation<SystemUser, { id: number; data: UserUpdateData }>({
      query: ({ id, data }) => ({ url: `/api/v1/users/${id}`, method: 'PUT', data }),
      invalidatesTags: (_result, _error, { id }) => [{ type: 'User', id }],
    }),
    deleteUser: builder.mutation<void, number>({
      query: (id) => ({ url: `/api/v1/users/${id}`, method: 'DELETE' }),
      invalidatesTags: (_result, _error, id) => [{ type: 'User', id }],
    }),
    updateUserStatus: builder.mutation<void, { id: number; status: number }>({
      query: ({ id, status }) => ({ url: `/api/v1/users/${id}/status`, method: 'PUT', data: { status } }),
      invalidatesTags: (_result, _error, { id }) => [{ type: 'User', id }],
    }),
    resetUserPassword: builder.mutation<void, { id: number; password: string; must_change?: boolean }>({
      query: ({ id, ...data }) => ({ url: `/api/v1/users/${id}/password`, method: 'PUT', data }),
      invalidatesTags: (_result, _error, { id }) => [{ type: 'User', id }],
    }),
    getRoleList: builder.query<PageResponse<Role>, RoleListParams>({
      query: (params) => ({ url: '/api/v1/roles', method: 'GET', params }),
      providesTags: [{ type: 'Role', id: 'LIST' }],
    }),
    getMenuList: builder.query<MenuItem[], void>({
      query: () => ({ url: '/api/v1/menus', method: 'GET' }),
      providesTags: [{ type: 'Menu', id: 'LIST' }],
    }),
    getPermissionTree: builder.query<Permission[], void>({
      query: () => ({ url: '/api/v1/permissions/tree', method: 'GET' }),
      providesTags: [{ type: 'Permission', id: 'LIST' }],
    }),
  }),
})

export const {
  useGetUserListQuery,
  useCreateUserMutation,
  useUpdateUserMutation,
  useDeleteUserMutation,
  useUpdateUserStatusMutation,
  useResetUserPasswordMutation,
  useGetRoleListQuery,
  useGetMenuListQuery,
  useGetPermissionTreeQuery,
} = systemApi
