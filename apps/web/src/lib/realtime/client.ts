import { browser } from '$app/environment';
import { get, writable } from 'svelte/store';

import {
  authSession,
  canAttemptSessionRefresh,
  refreshStoredSession,
} from '$lib/auth/client';

export type RealtimeStatus =
  | 'idle'
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'disconnected'
  | 'error';

export type RealtimeEvent = {
  channel: string;
  type: string;
  payload?: Record<string, unknown>;
  created_at: string;
};

export type RealtimeSnapshot = {
  status: RealtimeStatus;
  connection_id: string | null;
  channels: string[];
  events: RealtimeEvent[];
  last_heartbeat_at: string | null;
  reconnect_attempt: number;
  last_error: string | null;
};

type HelloFrame = {
  kind: 'hello';
  connection_id: string;
  user_id: string;
  role: string;
  channels: string[];
  heartbeat_interval_seconds: number;
  connected_at: string;
};

type EventFrame = {
  kind: 'event';
  event: RealtimeEvent;
};

type HeartbeatFrame = {
  kind: 'heartbeat';
  sent_at: string;
};

const maxEvents = 20;
const initialSnapshot: RealtimeSnapshot = {
  status: 'idle',
  connection_id: null,
  channels: [],
  events: [],
  last_heartbeat_at: null,
  reconnect_attempt: 0,
  last_error: null,
};

export const realtimeState = writable<RealtimeSnapshot>(initialSnapshot);

let socket: WebSocket | null = null;
let reconnectTimer: number | null = null;
let reconnectAttempt = 0;
let manualDisconnect = false;

export function connectRealtime() {
  if (!browser) {
    return;
  }

  clearReconnectTimer();
  manualDisconnect = false;

  if (
    socket &&
    (socket.readyState === WebSocket.OPEN ||
      socket.readyState === WebSocket.CONNECTING)
  ) {
    return;
  }

  const session = get(authSession);
  if (!session?.access_token) {
    realtimeState.set({ ...initialSnapshot });
    return;
  }

  const nextStatus: RealtimeStatus =
    reconnectAttempt > 0 ? 'reconnecting' : 'connecting';
  realtimeState.update((snapshot) => ({
    ...snapshot,
    status: nextStatus,
    reconnect_attempt: reconnectAttempt,
    last_error: null,
  }));

  const ws = new WebSocket(
    resolveRealtimeURL('/v1/realtime/ws', session.access_token),
  );
  socket = ws;

  ws.onopen = () => {
    reconnectAttempt = 0;
    realtimeState.update((snapshot) => ({
      ...snapshot,
      status: 'connected',
      reconnect_attempt: 0,
      last_error: null,
    }));
  };

  ws.onmessage = (message) => {
    const frame = parseFrame(message.data);
    if (!frame) {
      return;
    }

    if (frame.kind === 'hello') {
      realtimeState.update((snapshot) => ({
        ...snapshot,
        status: 'connected',
        connection_id: frame.connection_id,
        channels: frame.channels,
        last_error: null,
      }));
      return;
    }

    if (frame.kind === 'event') {
      realtimeState.update((snapshot) => ({
        ...snapshot,
        events: [frame.event, ...snapshot.events].slice(0, maxEvents),
      }));
      return;
    }

    realtimeState.update((snapshot) => ({
      ...snapshot,
      last_heartbeat_at: frame.sent_at,
    }));
  };

  ws.onerror = () => {
    realtimeState.update((snapshot) => ({
      ...snapshot,
      status: 'error',
      last_error: 'REALTIME_SOCKET_ERROR',
    }));
  };

  ws.onclose = () => {
    if (socket === ws) {
      socket = null;
    }

    if (manualDisconnect) {
      realtimeState.update((snapshot) => ({
        ...snapshot,
        status: 'disconnected',
      }));
      return;
    }

    scheduleReconnect();
  };
}

export function disconnectRealtime() {
  if (!browser) {
    return;
  }

  manualDisconnect = true;
  clearReconnectTimer();

  if (socket) {
    socket.close();
    socket = null;
  }

  reconnectAttempt = 0;
  realtimeState.set({ ...initialSnapshot, status: 'disconnected' });
}

export function sendRealtimePing() {
  if (!socket || socket.readyState !== WebSocket.OPEN) {
    return false;
  }

  socket.send(JSON.stringify({ type: 'ping' }));
  return true;
}

function scheduleReconnect() {
  if (!browser) {
    return;
  }

  clearReconnectTimer();
  reconnectAttempt += 1;
  const delay = Math.min(5000, 1000 * reconnectAttempt);

  realtimeState.update((snapshot) => ({
    ...snapshot,
    status: 'reconnecting',
    reconnect_attempt: reconnectAttempt,
  }));

  reconnectTimer = window.setTimeout(async () => {
    reconnectTimer = null;

    if (canAttemptSessionRefresh()) {
      await refreshStoredSession();
    }
    connectRealtime();
  }, delay);
}

function clearReconnectTimer() {
  if (reconnectTimer !== null) {
    window.clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
}

function parseFrame(
  payload: unknown,
): HelloFrame | EventFrame | HeartbeatFrame | null {
  if (typeof payload !== 'string') {
    return null;
  }

  try {
    const frame = JSON.parse(payload) as
      | HelloFrame
      | EventFrame
      | HeartbeatFrame;
    if (frame && typeof frame === 'object' && 'kind' in frame) {
      return frame;
    }
  } catch {
    return null;
  }

  return null;
}

function resolveRealtimeURL(path: string, accessToken: string) {
  const baseURL = (import.meta.env.PUBLIC_API_BASE_URL ?? '')
    .trim()
    .replace(/\/$/, '');
  const origin = baseURL === '' ? window.location.origin : baseURL;
  const url = new URL(path, origin);

  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  url.searchParams.set('access_token', accessToken);

  return url.toString();
}
