import { apiRequest } from '$lib/auth/client';

export type StoreMember = {
  id: string;
  store_id: string;
  real_username: string;
  upstream_user_code: string;
  status: 'active' | 'inactive';
  created_at: string;
  updated_at: string;
};

export async function fetchStoreMembers(storeID: string) {
  return apiRequest<StoreMember[]>(`/v1/stores/${storeID}/members`);
}

export async function createStoreMember(storeID: string, realUsername: string) {
  return apiRequest<StoreMember>(`/v1/stores/${storeID}/members`, {
    method: 'POST',
    body: {
      real_username: realUsername,
    },
  });
}
