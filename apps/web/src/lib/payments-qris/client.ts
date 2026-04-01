import { apiRequest } from '$lib/auth/client';

export type StoreTopup = {
  id: string;
  store_id: string;
  store_member_id: string | null;
  type: 'store_topup' | 'member_payment';
  provider_trx_id: string | null;
  custom_ref: string;
  external_username: string;
  amount_gross: string;
  platform_fee_amount: string;
  store_credit_amount: string;
  status: 'pending' | 'success' | 'expired' | 'failed';
  expires_at: string | null;
  provider_state?:
    | 'pending_generate'
    | 'generated'
    | 'pending_provider_response'
    | 'generate_failed'
    | null;
  qr_code_value?: string | null;
  created_at: string;
  updated_at: string;
};

export type StoreTopupSummary = {
  total_count: number;
  pending_count: number;
  success_count: number;
  expired_count: number;
  failed_count: number;
  total_gross: string;
  pending_gross: string;
};

export type StoreTopupPage = {
  items: StoreTopup[];
  summary: StoreTopupSummary;
  limit: number;
  offset: number;
};

export async function fetchStoreTopups(
  storeID: string,
  params: {
    type?: StoreTopup['type'];
    status?: StoreTopup['status'] | 'all';
    query?: string;
    limit?: number;
    offset?: number;
    createdFrom?: string;
    createdTo?: string;
  } = {},
) {
  const search = new URLSearchParams();
  if (params.type) {
    search.set('type', params.type);
  }
  if (params.status && params.status !== 'all') {
    search.set('status', params.status);
  }
  if ((params.query ?? '').trim() !== '') {
    search.set('query', params.query!.trim());
  }
  if (params.limit) {
    search.set('limit', String(params.limit));
  }
  if (params.offset && params.offset > 0) {
    search.set('offset', String(params.offset));
  }
  if ((params.createdFrom ?? '').trim() !== '') {
    search.set('created_from', new Date(params.createdFrom!).toISOString());
  }
  if ((params.createdTo ?? '').trim() !== '') {
    search.set('created_to', new Date(params.createdTo!).toISOString());
  }

  const suffix = search.toString();
  return apiRequest<StoreTopupPage>(
    `/v1/stores/${storeID}/topups/qris${suffix === '' ? '' : `?${suffix}`}`,
  );
}

export async function createStoreTopup(storeID: string, amount: number) {
  return apiRequest<StoreTopup>(`/v1/stores/${storeID}/topups/qris`, {
    method: 'POST',
    body: {
      amount,
    },
  });
}
