import { apiRequest } from '$lib/auth/client';

export type BankDirectoryEntry = {
  bank_code: string;
  bank_name: string;
  bank_swift_code: string;
};

export type BankAccount = {
  id: string;
  store_id: string;
  bank_code: string;
  bank_name: string;
  account_number_masked: string;
  account_name: string;
  verified_at: string | null;
  is_active: boolean;
  created_at: string;
  updated_at: string;
};

export async function searchBanks(query: string, limit = 20) {
  const search = new URLSearchParams();
  if (query.trim() !== '') {
    search.set('query', query.trim());
  }
  search.set('limit', String(limit));

  return apiRequest<BankDirectoryEntry[]>(`/v1/banks?${search.toString()}`);
}

export async function fetchBankAccounts(storeID: string) {
  return apiRequest<BankAccount[]>(`/v1/stores/${storeID}/bank-accounts`);
}

export async function createBankAccount(
  storeID: string,
  payload: {
    bank_code: string;
    account_number: string;
  },
) {
  return apiRequest<BankAccount>(`/v1/stores/${storeID}/bank-accounts`, {
    method: 'POST',
    body: payload,
  });
}

export async function updateBankAccountStatus(
  storeID: string,
  bankAccountID: string,
  isActive: boolean,
) {
  return apiRequest<BankAccount>(
    `/v1/stores/${storeID}/bank-accounts/${bankAccountID}`,
    {
      method: 'PATCH',
      body: {
        is_active: isActive,
      },
    },
  );
}
