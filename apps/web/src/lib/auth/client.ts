import { browser } from '$app/environment';
import { get, writable } from 'svelte/store';

const storageKey = 'onixggr.dashboard.auth';
const csrfCookieName = 'onixggr_csrf_token';

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

type RefreshAttemptResult = 'refreshed' | 'invalid' | 'unavailable';

let hydrated = false;
let initializePromise: Promise<AuthSession | null> | null = null;

export const authSession = writable<AuthSession | null>(null);

export function hydrateAuthSession() {
  if (!browser || hydrated) {
    return;
  }

  hydrated = true;

  const raw = window.sessionStorage.getItem(storageKey);
  if (!raw) {
    clearLegacyAuthStorage();
    authSession.set(null);
    return;
  }

  try {
    clearLegacyAuthStorage();
    authSession.set(JSON.parse(raw) as AuthSession);
  } catch {
    window.sessionStorage.removeItem(storageKey);
    clearLegacyAuthStorage();
    authSession.set(null);
  }
}

export async function initializeAuthSession() {
  hydrateAuthSession();

  const current = get(authSession);
  if (current) {
    return current;
  }

  if (!browser) {
    return null;
  }

  if (!canAttemptSessionRefresh()) {
    return null;
  }

  if (initializePromise) {
    return initializePromise;
  }

  initializePromise = refreshStoredSession().finally(() => {
    initializePromise = null;
  });

  return initializePromise;
}

export function saveAuthSession(session: AuthSession) {
  authSession.set(session);

  if (browser) {
    window.sessionStorage.setItem(storageKey, JSON.stringify(session));
    clearLegacyAuthStorage();
  }
}

export function clearAuthSession() {
  authSession.set(null);

  if (browser) {
    window.sessionStorage.removeItem(storageKey);
    clearLegacyAuthStorage();
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
  if (browser && !canAttemptSessionRefresh()) {
    return null;
  }

  const result = await attemptSessionRefresh();
  if (result.kind !== 'refreshed') {
    if (result.kind === 'invalid') {
      clearAuthSession();
    }
    return null;
  }

  saveAuthSession(result.session);
  return result.session;
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
  const method = options.method ?? 'GET';

  if (options.authenticated !== false && session?.access_token) {
    headers.set('Authorization', `Bearer ${session.access_token}`);
  }

  const csrfToken = mutationCSRFToken(method);
  if (csrfToken !== '') {
    headers.set('X-CSRF-Token', csrfToken);
  }

  const response = await fetch(resolveURL(path), {
    method,
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    credentials: 'include',
  });

  if (
    response.status === 401 &&
    options.authenticated !== false &&
    options.allowRefresh !== false &&
    retryOnUnauthorized
  ) {
    if (canAttemptSessionRefresh()) {
      const refreshResult = await attemptSessionRefresh();
      if (refreshResult.kind === 'refreshed') {
        saveAuthSession(refreshResult.session);
        return request<T>(path, { ...options, allowRefresh: false }, false);
      }

      if (refreshResult.kind === 'unavailable') {
        return {
          status: false,
          message: 'SESSION_RECOVERY_FAILED',
          data: null as T,
        };
      }
    }
  }

  return readEnvelope<T>(response);
}

async function attemptSessionRefresh(): Promise<
  | { kind: 'refreshed'; session: AuthSession }
  | { kind: Exclude<RefreshAttemptResult, 'refreshed'> }
> {
  const headers = new Headers({
    'Content-Type': 'application/json',
  });
  const csrfToken = mutationCSRFToken('POST');
  if (csrfToken !== '') {
    headers.set('X-CSRF-Token', csrfToken);
  }

  try {
    const response = await fetch(resolveURL('/v1/auth/refresh'), {
      method: 'POST',
      headers,
      credentials: 'include',
    });
    const envelope = await readEnvelope<AuthSession>(response);

    if (response.ok && envelope.status && envelope.message === 'SUCCESS') {
      return {
        kind: 'refreshed',
        session: envelope.data,
      };
    }

    if (response.status === 401) {
      return { kind: 'invalid' };
    }

    return { kind: 'unavailable' };
  } catch {
    return { kind: 'unavailable' };
  }
}

function mutationCSRFToken(method: string) {
  if (!browser) {
    return '';
  }

  switch (method.toUpperCase()) {
    case 'GET':
    case 'HEAD':
    case 'OPTIONS':
      return '';
    default:
      return readCookie(csrfCookieName);
  }
}

function readCookie(name: string) {
  if (!browser) {
    return '';
  }

  const pattern = `${name}=`;
  for (const chunk of document.cookie.split(';')) {
    const entry = chunk.trim();
    if (!entry.startsWith(pattern)) {
      continue;
    }

    return decodeURIComponent(entry.slice(pattern.length));
  }

  return '';
}

function clearLegacyAuthStorage() {
  if (!browser) {
    return;
  }

  window.localStorage.removeItem(storageKey);
}

export function canAttemptSessionRefresh() {
  if (!browser) {
    return false;
  }

  return readCookie(csrfCookieName) !== '';
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
