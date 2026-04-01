import { apiRequest } from '$lib/auth/client';

export type StoreWithdrawal = {
  id: string;
  store_id: string;
  store_bank_account_id: string;
  idempotency_key: string;
  bank_code: string;
  bank_name: string;
  account_name: string;
  account_number_masked: string;
  net_requested_amount: string;
  platform_fee_amount: string;
  external_fee_amount: string;
  total_store_debit: string;
  provider_partner_ref_no?: string | null;
  provider_inquiry_id?: string | null;
  status: 'pending' | 'success' | 'failed';
  created_at: string;
  updated_at: string;
};

export type StoreWithdrawalSummary = {
  total_count: number;
  pending_count: number;
  success_count: number;
  failed_count: number;
  total_net_amount: string;
  total_platform_fee: string;
  total_external_fee: string;
};

export type StoreWithdrawalPage = {
  items: StoreWithdrawal[];
  summary: StoreWithdrawalSummary;
  limit: number;
  offset: number;
};

export async function fetchStoreWithdrawals(
  storeID: string,
  params: {
    status?: StoreWithdrawal['status'] | 'all';
    query?: string;
    limit?: number;
    offset?: number;
    createdFrom?: string;
    createdTo?: string;
  } = {},
) {
  const search = new URLSearchParams();
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
  return apiRequest<StoreWithdrawalPage>(
    `/v1/stores/${storeID}/withdrawals${suffix === '' ? '' : `?${suffix}`}`,
  );
}

export async function createStoreWithdrawal(
  storeID: string,
  payload: {
    bank_account_id: string;
    amount: number;
    idempotency_key: string;
  },
) {
  return apiRequest<StoreWithdrawal | null>(
    `/v1/stores/${storeID}/withdrawals`,
    {
      method: 'POST',
      body: payload,
    },
  );
}
