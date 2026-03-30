import { browser } from '$app/environment';
import { get, writable } from 'svelte/store';

const storageKey = 'onixggr.dashboard.auth';

export type AuthUser = {
  id: string;
  email: string;
  username: string;
  role: string;
  totp_enabled: boolean;
  last_login_at: string | null;
};

export type AuthSession = {
  user: AuthUser;
  token_type: string;
  access_token: string;
  access_token_expires_at: string;
  refresh_token: string;
  refresh_token_expires_at: string;
  session_jti: string;
};

export type SecuritySettings = {
  user_id: string;
  totp_enabled: boolean;
  ip_allowlist: string | null;
  recommended_2fa: boolean;
};

export type TOTPEnrollment = {
  secret: string;
  otpauth_url: string;
  expires_at: string;
};

export type ApiEnvelope<T> = {
  status: boolean;
  message: string;
  data: T;
};

export type RequestOptions = {
  method?: string;
  body?: unknown;
  authenticated?: boolean;
  allowRefresh?: boolean;
};

let hydrated = false;

export const authSession = writable<AuthSession | null>(null);

export function hydrateAuthSession() {
  if (!browser || hydrated) {
    return;
  }

  hydrated = true;

  const raw = window.localStorage.getItem(storageKey);
  if (!raw) {
    authSession.set(null);
    return;
  }

  try {
    authSession.set(JSON.parse(raw) as AuthSession);
  } catch {
    window.localStorage.removeItem(storageKey);
    authSession.set(null);
  }
}

export function saveAuthSession(session: AuthSession) {
  authSession.set(session);

  if (browser) {
    window.localStorage.setItem(storageKey, JSON.stringify(session));
  }
}

export function clearAuthSession() {
  authSession.set(null);

  if (browser) {
    window.localStorage.removeItem(storageKey);
  }
}

export async function login(payload: {
  login: string;
  password: string;
  totp_code?: string;
  recovery_code?: string;
}) {
  return request<AuthSession>('/v1/auth/login', {
    method: 'POST',
    body: payload,
    authenticated: false,
  });
}

export async function refreshStoredSession() {
  const session = get(authSession);
  if (!session) {
    return null;
  }

  const response = await request<AuthSession>(
    '/v1/auth/refresh',
    {
      method: 'POST',
      body: {
        refresh_token: session.refresh_token,
      },
      authenticated: false,
      allowRefresh: false,
    },
    false,
  );

  if (!response.status || response.message !== 'SUCCESS') {
    clearAuthSession();
    return null;
  }

  saveAuthSession(response.data);
  return response.data;
}

export async function fetchProfile() {
  return request<AuthUser>('/v1/auth/me');
}

export async function fetchSecuritySettings() {
  return request<SecuritySettings>('/v1/auth/security');
}

export async function beginTOTPEnrollment() {
  return request<TOTPEnrollment>('/v1/auth/2fa/enroll', {
    method: 'POST',
  });
}

export async function enableTOTP(code: string) {
  return request<{ codes: string[] }>('/v1/auth/2fa/enable', {
    method: 'POST',
    body: { code },
  });
}

export async function disableTOTP(payload: {
  totp_code?: string;
  recovery_code?: string;
}) {
  return request<null>('/v1/auth/2fa/disable', {
    method: 'POST',
    body: payload,
  });
}

export async function updateIPAllowlist(ipAllowlist: string | null) {
  return request<SecuritySettings>('/v1/auth/ip-allowlist', {
    method: 'PUT',
    body: { ip_allowlist: ipAllowlist },
  });
}

export async function logoutCurrentSession() {
  const response = await request<null>('/v1/auth/logout', {
    method: 'POST',
  });

  clearAuthSession();
  return response;
}

export async function logoutAllSessions() {
  const response = await request<{ revoked_sessions: number }>(
    '/v1/auth/logout-all',
    {
      method: 'POST',
    },
  );

  clearAuthSession();
  return response;
}

export async function syncProfile() {
  const response = await fetchProfile();
  if (!response.status || response.message !== 'SUCCESS') {
    return response;
  }

  const session = get(authSession);
  if (!session) {
    return response;
  }

  saveAuthSession({
    ...session,
    user: response.data,
  });

  return response;
}

export async function apiRequest<T>(
  path: string,
  options: RequestOptions = {},
  retryOnUnauthorized = true,
) {
  return request<T>(path, options, retryOnUnauthorized);
}

async function request<T>(
  path: string,
  options: RequestOptions = {},
  retryOnUnauthorized = true,
): Promise<ApiEnvelope<T>> {
  const session = get(authSession);
  const headers = new Headers({
    'Content-Type': 'application/json',
  });

  if (options.authenticated !== false && session?.access_token) {
    headers.set('Authorization', `Bearer ${session.access_token}`);
  }

  const response = await fetch(resolveURL(path), {
    method: options.method ?? 'GET',
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });

  if (
    response.status === 401 &&
    options.authenticated !== false &&
    options.allowRefresh !== false &&
    retryOnUnauthorized
  ) {
    const refreshed = await refreshStoredSession();
    if (refreshed) {
      return request<T>(path, { ...options, allowRefresh: false }, false);
    }
  }

  return readEnvelope<T>(response);
}

async function readEnvelope<T>(response: Response): Promise<ApiEnvelope<T>> {
  try {
    return (await response.json()) as ApiEnvelope<T>;
  } catch {
    return {
      status: false,
      message: response.ok ? 'UNKNOWN_RESPONSE' : 'NETWORK_ERROR',
      data: null as T,
    };
  }
}

function resolveURL(path: string) {
  const baseURL = (import.meta.env.PUBLIC_API_BASE_URL ?? '')
    .trim()
    .replace(/\/$/, '');
  if (baseURL === '') {
    return path;
  }

  return `${baseURL}${path}`;
}
