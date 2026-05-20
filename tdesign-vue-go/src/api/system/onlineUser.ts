import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type OnlineUserItem = Schema<'OnlineUserItem'>;
export type OnlineUserListResponse = Schema<'OnlineUserListResponse'>;
export type OnlineUserCountResponse = Schema<'OnlineUserCountResponse'>;

export function getOnlineUsers() {
  return typedApi.get('/api/v1/online-users');
}

export function getOnlineUserCount() {
  return typedApi.get('/api/v1/online-users/count');
}

export function forceLogout(tokenId: string) {
  return typedApi.delete('/api/v1/online-users/{token_id}', {
    path: { token_id: tokenId },
  });
}
