import { browser } from '$app/environment';

import { apiRequest } from '$lib/auth/client';

export type NotificationScopeType = 'store' | 'role' | 'user' | 'global';
export type NotificationScopeMode = 'auto' | 'store' | 'role';

export type NotificationRecord = {
  id: string;
  scope_type: NotificationScopeType;
  scope_id: string;
  event_type: string;
  title: string;
  body: string;
  read_at: string | null;
  created_at: string;
};

export type NotificationQuery = {
  storeID?: string;
  scopeType?: NotificationScopeType;
  scopeID?: string;
  limit?: number;
  offset?: number;
};

export type ResolvedNotificationScope = {
  key: string;
  label: string;
  description: string;
  ready: boolean;
  channel: string;
  params: NotificationQuery;
};

const changedEventName = 'onixggr:notifications-changed';
const notificationEventTypes = new Set([
  'member_payment.success',
  'store_topup.success',
  'withdraw.success',
  'withdraw.failed',
  'callback.delivery_failed',
  'game.deposit.success',
  'game.withdraw.success',
  'store.low_balance',
]);

export async function fetchNotifications(params: NotificationQuery = {}) {
  return apiRequest<NotificationRecord[]>(
    `/v1/notifications${buildQueryString(params)}`,
  );
}

export async function fetchUnreadNotificationCount(
  params: NotificationQuery = {},
) {
  return apiRequest<{ unread_count: number }>(
    `/v1/notifications/unread-count${buildQueryString(params)}`,
  );
}

export async function markNotificationRead(
  notificationID: string,
  params: NotificationQuery = {},
) {
  return apiRequest<null>(
    `/v1/notifications/${notificationID}/read${buildQueryString(params)}`,
    {
      method: 'POST',
    },
  );
}

export function resolveNotificationScope(
  role: string,
  storeID: string,
  mode: NotificationScopeMode = 'auto',
): ResolvedNotificationScope {
  const normalizedRole = role.trim();
  const normalizedStoreID = storeID.trim();

  if (normalizedRole === 'owner' || normalizedRole === 'karyawan') {
    if (normalizedStoreID === '') {
      return {
        key: `${normalizedRole}:missing-store`,
        label: 'Store Scope',
        description:
          'Pilih store aktif dari sidebar untuk membuka feed notifikasi.',
        ready: false,
        channel: '',
        params: {},
      };
    }

    return {
      key: `store:${normalizedStoreID}`,
      label: 'Store Scope',
      description:
        'Feed notifikasi mengikuti store aktif yang dipilih di app shell.',
      ready: true,
      channel: `store:${normalizedStoreID}`,
      params: { storeID: normalizedStoreID },
    };
  }

  if (normalizedRole === 'dev' || normalizedRole === 'superadmin') {
    if (mode === 'store' || (mode === 'auto' && normalizedStoreID !== '')) {
      if (normalizedStoreID === '') {
        return {
          key: `${normalizedRole}:missing-store`,
          label: 'Store Scope',
          description:
            'Pilih store aktif dari sidebar atau pindah ke platform stream.',
          ready: false,
          channel: '',
          params: {},
        };
      }

      return {
        key: `store:${normalizedStoreID}`,
        label: 'Store Scope',
        description:
          'Sebagai role platform, feed ini membaca stream toko aktif yang dipilih di shell.',
        ready: true,
        channel: `store:${normalizedStoreID}`,
        params: {
          scopeType: 'store',
          scopeID: normalizedStoreID,
        },
      };
    }

    return {
      key: `role:${normalizedRole}`,
      label: 'Platform Scope',
      description:
        'Feed ini membaca scope role platform dan menampilkan notification operasional lintas store untuk dev atau superadmin.',
      ready: true,
      channel: `role:${normalizedRole}`,
      params: {
        scopeType: 'role',
        scopeID: normalizedRole,
      },
    };
  }

  return {
    key: `${normalizedRole}:unsupported`,
    label: 'Unsupported Scope',
    description:
      'Role sesi ini tidak memiliki akses ke notification feed dashboard.',
    ready: false,
    channel: '',
    params: {},
  };
}

export function isNotificationEvent(eventType: string) {
  return notificationEventTypes.has(eventType.trim());
}

export function notifyNotificationsChanged() {
  if (!browser) {
    return;
  }

  window.dispatchEvent(new CustomEvent(changedEventName));
}

export function subscribeNotificationsChanged(callback: () => void) {
  if (!browser) {
    return () => {};
  }

  window.addEventListener(changedEventName, callback);
  return () => window.removeEventListener(changedEventName, callback);
}

function buildQueryString(params: NotificationQuery) {
  const query = new URLSearchParams();

  if ((params.storeID ?? '').trim() !== '') {
    query.set('store_id', (params.storeID ?? '').trim());
  } else {
    if ((params.scopeType ?? '').trim() !== '') {
      query.set('scope_type', (params.scopeType ?? '').trim());
    }
    if ((params.scopeID ?? '').trim() !== '') {
      query.set('scope_id', (params.scopeID ?? '').trim());
    }
  }

  if ((params.limit ?? 0) > 0) {
    query.set('limit', String(params.limit));
  }

  if ((params.offset ?? 0) > 0) {
    query.set('offset', String(params.offset));
  }

  return query.size > 0 ? `?${query.toString()}` : '';
}
