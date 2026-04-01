import { apiRequest } from '$lib/auth/client';

export type ChatMessage = {
  id: string;
  sender_user_id: string;
  sender_username: string;
  sender_role: string;
  body: string;
  deleted_by_dev_user_id?: string | null;
  deleted_at?: string | null;
  created_at: string;
};

export type ChatQuery = {
  query?: string;
  role?: string;
  createdFrom?: string;
  createdTo?: string;
  limit?: number;
  offset?: number;
};

export type ChatMessagePage = {
  items: ChatMessage[];
  total_count: number;
};

export async function fetchChatMessages(params: ChatQuery = {}) {
  const search = new URLSearchParams();

  if ((params.query ?? '').trim() !== '') {
    search.set('query', (params.query ?? '').trim());
  }
  if ((params.role ?? '').trim() !== '' && params.role !== 'all') {
    search.set('role', (params.role ?? '').trim());
  }
  if ((params.createdFrom ?? '').trim() !== '') {
    search.set('created_from', (params.createdFrom ?? '').trim());
  }
  if ((params.createdTo ?? '').trim() !== '') {
    search.set('created_to', (params.createdTo ?? '').trim());
  }
  if ((params.limit ?? 0) > 0) {
    search.set('limit', String(params.limit));
  }
  if ((params.offset ?? 0) > 0) {
    search.set('offset', String(params.offset));
  }

  const suffix = search.size > 0 ? `?${search.toString()}` : '';
  return apiRequest<ChatMessagePage>(`/v1/chat/messages${suffix}`);
}

export async function sendChatMessage(body: string) {
  return apiRequest<ChatMessage>('/v1/chat/messages', {
    method: 'POST',
    body: { body },
  });
}

export async function deleteChatMessage(messageID: string) {
  return apiRequest<ChatMessage>(`/v1/chat/messages/${messageID}`, {
    method: 'DELETE',
  });
}
