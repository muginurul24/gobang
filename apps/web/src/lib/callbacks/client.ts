import { apiRequest } from '$lib/auth/client';

export type CallbackStatus = 'pending' | 'retrying' | 'success' | 'failed';
export type CallbackAttemptStatus = 'success' | 'failed';

export type CallbackQueueItem = {
  id: string;
  store_id: string;
  store_name: string;
  store_slug: string;
  callback_url: string;
  event_type: string;
  reference_type: string;
  reference_id: string;
  status: CallbackStatus;
  created_at: string;
  updated_at: string;
  latest_attempt_no: number;
  latest_http_status: number | null;
  latest_attempt_status: CallbackAttemptStatus | null;
  latest_response_body_masked: string;
  latest_next_retry_at: string | null;
  latest_attempt_at: string | null;
};

export type CallbackQueueSummary = {
  total_count: number;
  pending_count: number;
  retrying_count: number;
  success_count: number;
  failed_count: number;
};

export type CallbackQueuePage = {
  items: CallbackQueueItem[];
  summary: CallbackQueueSummary;
  limit: number;
  offset: number;
};

export type CallbackAttemptRecord = {
  id: string;
  outbound_callback_id: string;
  attempt_no: number;
  http_status: number | null;
  status: CallbackAttemptStatus;
  response_body_masked: string;
  next_retry_at: string | null;
  created_at: string;
};

export type CallbackAttemptPage = {
  callback_id: string;
  items: CallbackAttemptRecord[];
  total_count: number;
  limit: number;
  offset: number;
};

export type CallbackQueueQuery = {
  query?: string;
  status?: 'all' | CallbackStatus;
  storeID?: string;
  createdFrom?: string;
  createdTo?: string;
  limit?: number;
  offset?: number;
};

export async function fetchCallbackQueue(params: CallbackQueueQuery = {}) {
  return apiRequest<CallbackQueuePage>(
    `/v1/callbacks/queue${buildQueueQuery(params)}`,
  );
}

export async function fetchCallbackAttempts(
  callbackID: string,
  params: Pick<CallbackQueueQuery, 'limit' | 'offset'> = {},
) {
  return apiRequest<CallbackAttemptPage>(
    `/v1/callbacks/${callbackID}/attempts${buildAttemptsQuery(params)}`,
  );
}

function buildQueueQuery(params: CallbackQueueQuery) {
  const search = new URLSearchParams();

  if ((params.query ?? '').trim() !== '') {
    search.set('query', (params.query ?? '').trim());
  }
  if ((params.status ?? 'all') !== 'all') {
    search.set('status', (params.status ?? 'all').trim());
  }
  if ((params.storeID ?? '').trim() !== '') {
    search.set('store_id', (params.storeID ?? '').trim());
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

function buildAttemptsQuery(
  params: Pick<CallbackQueueQuery, 'limit' | 'offset'>,
) {
  const search = new URLSearchParams();

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

  const parsed = new Date(trimmed);
  if (Number.isNaN(parsed.getTime())) {
    return '';
  }

  return parsed.toISOString();
}
