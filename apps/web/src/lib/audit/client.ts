import { apiRequest } from '$lib/auth/client';

export type AuditLogEntry = {
  id: string;
  actor_user_id: string | null;
  actor_role: string;
  store_id: string | null;
  action: string;
  target_type: string;
  target_id: string | null;
  payload_masked: Record<string, unknown> | null;
  ip_address: string | null;
  user_agent: string | null;
  created_at: string;
};

export async function fetchAuditLogs(
  params: {
    storeID?: string;
    limit?: number;
    action?: string;
    actorRole?: string;
    targetType?: string;
  } = {},
) {
  const search = new URLSearchParams();
  if (params.storeID) {
    search.set('store_id', params.storeID);
  }
  if (params.action) {
    search.set('action', params.action);
  }
  if (params.actorRole) {
    search.set('actor_role', params.actorRole);
  }
  if (params.targetType) {
    search.set('target_type', params.targetType);
  }

  if (params.limit) {
    search.set('limit', String(params.limit));
  }

  const suffix = search.size > 0 ? `?${search.toString()}` : '';
  return apiRequest<AuditLogEntry[]>(`/v1/audit/logs${suffix}`);
}
