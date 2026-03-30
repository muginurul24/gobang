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

export async function fetchStoreTopups(storeID: string) {
  return apiRequest<StoreTopup[]>(`/v1/stores/${storeID}/topups/qris`);
}

export async function createStoreTopup(storeID: string, amount: number) {
  return apiRequest<StoreTopup>(`/v1/stores/${storeID}/topups/qris`, {
    method: 'POST',
    body: {
      amount,
    },
  });
}
