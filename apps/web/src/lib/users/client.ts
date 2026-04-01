import { apiRequest } from '$lib/auth/client';

export type ManagedUserRole = 'dev' | 'superadmin' | 'owner';

export type ManagedUser = {
  id: string;
  email: string;
  username: string;
  role: ManagedUserRole;
  is_active: boolean;
  created_by_user_id: string | null;
  created_at: string;
  updated_at: string;
  last_login_at: string | null;
};

export type UserDirectorySummary = {
  total_count: number;
  owner_count: number;
  superadmin_count: number;
  dev_count: number;
  active_count: number;
  inactive_count: number;
};

export type UserDirectoryPage = {
  items: ManagedUser[];
  summary: UserDirectorySummary;
  limit: number;
  offset: number;
};

export type UserDirectoryQuery = {
  query?: string;
  role?: '' | 'all' | ManagedUserRole;
  isActive?: '' | 'all' | 'true' | 'false';
  createdFrom?: string;
  createdTo?: string;
  limit?: number;
  offset?: number;
};

export async function fetchUserDirectory(params: UserDirectoryQuery = {}) {
  return apiRequest<UserDirectoryPage>(
    `/v1/users/directory${buildUserDirectoryQuery(params)}`,
  );
}

export async function createManagedUser(payload: {
  email: string;
  username: string;
  password: string;
  role: ManagedUserRole | 'owner' | 'superadmin';
}) {
  return apiRequest<ManagedUser>('/v1/users', {
    method: 'POST',
    body: payload,
  });
}

export async function updateManagedUserStatus(
  userID: string,
  isActive: boolean,
) {
  return apiRequest<ManagedUser>(`/v1/users/${userID}/status`, {
    method: 'PATCH',
    body: {
      is_active: isActive,
    },
  });
}

function buildUserDirectoryQuery(params: UserDirectoryQuery) {
  const search = new URLSearchParams();

  if ((params.query ?? '').trim() !== '') {
    search.set('query', (params.query ?? '').trim());
  }
  if ((params.role ?? 'all') !== 'all' && (params.role ?? '') !== '') {
    search.set('role', params.role ?? '');
  }
  if ((params.isActive ?? 'all') !== 'all' && (params.isActive ?? '') !== '') {
    search.set('is_active', params.isActive ?? '');
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

function normalizeDateTimeParam(value: string) {
  const trimmed = value.trim();
  if (trimmed === '') {
    return '';
  }

  if (/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) {
    return `${trimmed}T00:00`;
  }

  return trimmed;
}
