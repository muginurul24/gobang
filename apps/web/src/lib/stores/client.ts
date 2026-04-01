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

export type StoreDirectorySummary = {
  total_count: number;
  active_count: number;
  inactive_count: number;
  banned_count: number;
  deleted_count: number;
  low_balance_count: number;
};

export type StorePage = {
  items: Store[];
  summary: StoreDirectorySummary;
  limit: number;
  offset: number;
};

export function parseMoney(value: string | number | null | undefined) {
  const amount = Number(value ?? 0);
  return Number.isFinite(amount) ? amount : 0;
}

export function isStoreLowBalance(
  store: Pick<Store, 'current_balance' | 'low_balance_threshold'>,
) {
  const threshold = parseMoney(store.low_balance_threshold);
  if (threshold <= 0) {
    return false;
  }

  return parseMoney(store.current_balance) <= threshold;
}

export type StaffUser = {
  id: string;
  email: string;
  username: string;
  role: string;
  created_by_user_id: string | null;
  created_at: string;
  last_login_at: string | null;
  assigned_at?: string | null;
};

export type StaffUserPage = {
  items: StaffUser[];
  total_count: number;
  limit: number;
  offset: number;
};

export type StoreDirectoryQuery = {
  query?: string;
  status?: string;
  lowBalanceState?: 'all' | 'low_balance' | 'healthy';
  createdFrom?: string;
  createdTo?: string;
  limit?: number;
  offset?: number;
};

export type StaffDirectoryQuery = {
  query?: string;
  createdFrom?: string;
  createdTo?: string;
  assignedFrom?: string;
  assignedTo?: string;
  limit?: number;
  offset?: number;
};

export async function fetchStores() {
  return apiRequest<Store[]>('/v1/stores');
}

export async function fetchStore(storeID: string) {
  return apiRequest<Store>(`/v1/stores/${storeID}`);
}

export async function fetchStoreDirectory(params: StoreDirectoryQuery = {}) {
  return apiRequest<StorePage>(
    `/v1/stores/directory${buildStoreDirectoryQuery(params)}`,
  );
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

export async function fetchEmployeeDirectory(params: StaffDirectoryQuery = {}) {
  return apiRequest<StaffUserPage>(
    `/v1/staff/users/directory${buildStaffDirectoryQuery(params)}`,
  );
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

export async function fetchStoreStaffDirectory(
  storeID: string,
  params: StaffDirectoryQuery = {},
) {
  return apiRequest<StaffUserPage>(
    `/v1/stores/${storeID}/staff/directory${buildStaffDirectoryQuery(params, true)}`,
  );
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

function buildStoreDirectoryQuery(params: StoreDirectoryQuery) {
  const search = new URLSearchParams();

  if ((params.query ?? '').trim() !== '') {
    search.set('query', (params.query ?? '').trim());
  }
  if ((params.status ?? '').trim() !== '' && params.status !== 'all') {
    search.set('status', (params.status ?? '').trim());
  }
  if ((params.lowBalanceState ?? 'all') !== 'all') {
    search.set('low_balance_state', params.lowBalanceState ?? 'all');
  }
  if ((params.createdFrom ?? '').trim() !== '') {
    search.set(
      'created_from',
      normalizeDateTimeParam(params.createdFrom ?? ''),
    );
  }
  if ((params.createdTo ?? '').trim() !== '') {
    search.set('created_to', normalizeDateTimeParam(params.createdTo ?? ''));
  }
  if ((params.limit ?? 0) > 0) {
    search.set('limit', String(params.limit));
  }
  if ((params.offset ?? 0) > 0) {
    search.set('offset', String(params.offset));
  }

  return search.size > 0 ? `?${search.toString()}` : '';
}

function buildStaffDirectoryQuery(
  params: StaffDirectoryQuery,
  preferAssignedWindow = false,
) {
  const search = new URLSearchParams();

  if ((params.query ?? '').trim() !== '') {
    search.set('query', (params.query ?? '').trim());
  }
  if ((params.limit ?? 0) > 0) {
    search.set('limit', String(params.limit));
  }
  if ((params.offset ?? 0) > 0) {
    search.set('offset', String(params.offset));
  }

  const from = normalizeDateTimeParam(
    preferAssignedWindow
      ? (params.assignedFrom ?? '')
      : (params.createdFrom ?? ''),
  );
  const to = normalizeDateTimeParam(
    preferAssignedWindow ? (params.assignedTo ?? '') : (params.createdTo ?? ''),
  );
  if (from !== '') {
    search.set(preferAssignedWindow ? 'assigned_from' : 'created_from', from);
  }
  if (to !== '') {
    search.set(preferAssignedWindow ? 'assigned_to' : 'created_to', to);
  }

  return search.size > 0 ? `?${search.toString()}` : '';
}

function normalizeDateTimeParam(value: string) {
  const trimmed = value.trim();
  if (trimmed === '') {
    return '';
  }

  const parsed = new Date(trimmed);
  if (Number.isNaN(parsed.getTime())) {
    return '';
  }

  return parsed.toISOString();
}
