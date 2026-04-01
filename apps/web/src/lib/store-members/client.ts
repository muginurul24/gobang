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

export type StoreMemberSummary = {
  total_count: number;
  active_count: number;
  inactive_count: number;
};

export type StoreMemberPage = {
  items: StoreMember[];
  summary: StoreMemberSummary;
  limit: number;
  offset: number;
};

export async function fetchStoreMembers(
  storeID: string,
  params: {
    query?: string;
    status?: 'active' | 'inactive';
    limit?: number;
    offset?: number;
    createdFrom?: string;
    createdTo?: string;
  } = {},
) {
  const search = new URLSearchParams();
  if (params.query?.trim()) {
    search.set('query', params.query.trim());
  }
  if (params.status?.trim()) {
    search.set('status', params.status.trim());
  }
  if (params.limit !== undefined) {
    search.set('limit', String(params.limit));
  }
  if (params.offset !== undefined) {
    search.set('offset', String(params.offset));
  }
  if (params.createdFrom?.trim()) {
    search.set('created_from', params.createdFrom);
  }
  if (params.createdTo?.trim()) {
    search.set('created_to', params.createdTo);
  }

  const suffix = search.size > 0 ? `?${search.toString()}` : '';
  return apiRequest<StoreMemberPage>(`/v1/stores/${storeID}/members${suffix}`);
}

export async function createStoreMember(storeID: string, realUsername: string) {
  return apiRequest<StoreMember>(`/v1/stores/${storeID}/members`, {
    method: 'POST',
    body: {
      real_username: realUsername,
    },
  });
}
