import { apiRequest } from '$lib/auth/client';

export type Store = {
  id: string;
  owner_user_id: string;
  name: string;
  slug: string;
  status: 'active' | 'inactive' | 'banned' | 'deleted';
  api_token?: string | null;
  callback_url: string;
  current_balance: string;
  low_balance_threshold: string | null;
  staff_count: number;
  created_at: string;
  updated_at: string;
  deleted_at: string | null;
};

export type StaffUser = {
  id: string;
  email: string;
  username: string;
  role: string;
  created_by_user_id: string | null;
  created_at: string;
  last_login_at: string | null;
};

export async function fetchStores() {
  return apiRequest<Store[]>('/v1/stores');
}

export async function createStore(payload: {
  name: string;
  slug: string;
  low_balance_threshold?: string;
}) {
  return apiRequest<Store>('/v1/stores', {
    method: 'POST',
    body: payload,
  });
}

export async function updateStore(
  storeID: string,
  payload: {
    name?: string;
    status?: string;
    low_balance_threshold?: string;
  },
) {
  return apiRequest<Store>(`/v1/stores/${storeID}`, {
    method: 'PATCH',
    body: payload,
  });
}

export async function deleteStore(storeID: string) {
  return apiRequest<null>(`/v1/stores/${storeID}`, {
    method: 'DELETE',
  });
}

export async function rotateStoreToken(storeID: string) {
  return apiRequest<{ token: string }>(`/v1/stores/${storeID}/token`, {
    method: 'POST',
  });
}

export async function updateStoreCallbackURL(
  storeID: string,
  callbackURL: string,
) {
  return apiRequest<Store>(`/v1/stores/${storeID}/callback-url`, {
    method: 'PUT',
    body: {
      callback_url: callbackURL,
    },
  });
}

export async function fetchEmployees() {
  return apiRequest<StaffUser[]>('/v1/staff/users');
}

export async function createEmployee(payload: {
  email: string;
  username: string;
  password: string;
}) {
  return apiRequest<StaffUser>('/v1/staff/users', {
    method: 'POST',
    body: payload,
  });
}

export async function fetchStoreStaff(storeID: string) {
  return apiRequest<StaffUser[]>(`/v1/stores/${storeID}/staff`);
}

export async function assignStoreStaff(storeID: string, userID: string) {
  return apiRequest<StaffUser[]>(`/v1/stores/${storeID}/staff`, {
    method: 'POST',
    body: { user_id: userID },
  });
}

export async function unassignStoreStaff(storeID: string, userID: string) {
  return apiRequest<StaffUser[]>(`/v1/stores/${storeID}/staff/${userID}`, {
    method: 'DELETE',
  });
}
