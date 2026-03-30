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

export async function fetchChatMessages(limit = 100) {
  return apiRequest<ChatMessage[]>(`/v1/chat/messages?limit=${limit}`);
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
