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

export async function fetchStoreWithdrawals(storeID: string) {
  return apiRequest<StoreWithdrawal[]>(`/v1/stores/${storeID}/withdrawals`);
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
