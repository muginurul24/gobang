<svelte:head>
  <title>Chat | onixggr</title>
</svelte:head>

<script lang="ts">
  import { onMount } from 'svelte';

  import { authSession } from '$lib/auth/client';
  import type { ChatMessage } from '$lib/chat/client';
  import {
    deleteChatMessage,
    fetchChatMessages,
    sendChatMessage
  } from '$lib/chat/client';
  import Button from '$lib/components/ui/button/button.svelte';
  import { realtimeState } from '$lib/realtime/client';

  let messages: ChatMessage[] = [];
  let body = '';
  let loading = true;
  let sending = false;
  let deletingID: string | null = null;
  let errorMessage: string | null = null;
  let lastRealtimeKey: string | null = null;

  $: role = $authSession?.user.role ?? '';
  $: canModerate = role === 'dev';
  $: channelReady = $realtimeState.channels.includes('global_chat');

  onMount(() => {
    let active = true;

    async function loadMessages() {
      const response = await fetchChatMessages();
      if (!active) {
        return;
      }

      if (!response.status || response.message !== 'SUCCESS') {
        errorMessage = response.message;
        loading = false;
        return;
      }

      messages = response.data;
      loading = false;
      errorMessage = null;
    }

    const unsubscribe = realtimeState.subscribe((snapshot) => {
      if (!active) {
        return;
      }

      const latestEvent = snapshot.events[0];
      if (!latestEvent || latestEvent.channel !== 'global_chat') {
        return;
      }

      const eventKey = `${latestEvent.created_at}:${latestEvent.type}`;
      if (eventKey === lastRealtimeKey) {
        return;
      }
      lastRealtimeKey = eventKey;

      if (latestEvent.type === 'chat.message.created') {
        const payload = latestEvent.payload as { message?: ChatMessage } | undefined;
        if (!payload?.message) {
          return;
        }

        if (messages.some((item) => item.id === payload.message?.id)) {
          return;
        }

        messages = [...messages, payload.message];
        return;
      }

      if (latestEvent.type === 'chat.message.deleted') {
        const payload = latestEvent.payload as { message_id?: string } | undefined;
        if (!payload?.message_id) {
          return;
        }

        messages = messages.filter((item) => item.id !== payload.message_id);
      }
    });

    void loadMessages();

    return () => {
      active = false;
      unsubscribe();
    };
  });

  async function submitMessage() {
    if (sending || body.trim() === '') {
      return;
    }

    sending = true;
    errorMessage = null;

    const response = await sendChatMessage(body);
    sending = false;

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = response.message;
      return;
    }

    body = '';
  }

  async function moderateDelete(messageID: string) {
    if (!canModerate || deletingID !== null) {
      return;
    }

    deletingID = messageID;
    errorMessage = null;

    const response = await deleteChatMessage(messageID);
    deletingID = null;
    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = response.message;
    }
  }

  function formatTimestamp(value: string) {
    return new Intl.DateTimeFormat('id-ID', {
      day: '2-digit',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit'
    }).format(new Date(value));
  }
</script>

<section class="glass-panel overflow-hidden rounded-4xl">
  <header class="border-b border-ink-100 px-6 py-5">
    <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">Global Chat</p>
    <h2 class="mt-2 font-display text-3xl font-bold tracking-tight text-ink-900">
      Satu room global untuk semua user dashboard, dengan moderasi delete oleh dev.
    </h2>
  </header>

  <div class="grid gap-4 p-6 lg:grid-cols-[1fr_18rem]">
    <div class="space-y-3">
      <article class="rounded-3xl bg-canvas-100 px-5 py-4 text-sm leading-6 text-ink-700">
        Chat ini hanya punya satu room: `global_chat`. Tidak ada DM, tidak ada edit, dan history
        dibersihkan otomatis setelah 7 hari.
      </article>
      <article class="rounded-3xl bg-ink-900 px-5 py-4 text-sm leading-6 text-white">
        Pengiriman message lewat HTTP, distribusi realtime lewat WebSocket `global_chat`. Saat dev
        menghapus message, item akan hilang untuk semua peserta.
      </article>

      {#if errorMessage}
        <article class="rounded-3xl border border-danger/30 bg-danger/10 px-5 py-4 text-sm text-danger">
          {errorMessage}
        </article>
      {/if}

      <article class="rounded-3xl border border-ink-100 px-5 py-4 text-sm leading-6 text-ink-700">
        <div class="flex items-center justify-between gap-4">
          <div>
            <p class="font-semibold text-ink-900">Room Timeline</p>
            <p class="mt-1 text-xs text-ink-500">
              {messages.length} message dalam retensi aktif
            </p>
          </div>
          <p class="text-xs uppercase tracking-[0.18em] text-brand-700">
            {$realtimeState.status}
          </p>
        </div>

        <div class="mt-4 max-h-[32rem] space-y-3 overflow-y-auto pr-1">
          {#if loading}
            <p class="rounded-2xl bg-canvas-100 px-4 py-3 text-sm text-ink-600">
              Memuat room global...
            </p>
          {:else if messages.length === 0}
            <p class="rounded-2xl bg-canvas-100 px-4 py-3 text-sm text-ink-600">
              Belum ada message di room global. Kirim message pertama untuk memulai.
            </p>
          {:else}
            {#each messages as message}
              <div class="rounded-2xl bg-canvas-100 px-4 py-4">
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <p class="text-sm font-semibold text-ink-900">{message.sender_username}</p>
                    <p class="text-xs uppercase tracking-[0.18em] text-ink-500">
                      {message.sender_role} • {formatTimestamp(message.created_at)}
                    </p>
                  </div>
                  {#if canModerate}
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={deletingID === message.id}
                      onclick={() => moderateDelete(message.id)}
                    >
                      {deletingID === message.id ? 'Deleting...' : 'Delete'}
                    </Button>
                  {/if}
                </div>
                <p class="mt-3 whitespace-pre-wrap text-sm leading-6 text-ink-800">{message.body}</p>
              </div>
            {/each}
          {/if}
        </div>
      </article>

      <form
        class="rounded-3xl border border-ink-100 px-5 py-4"
        on:submit|preventDefault={submitMessage}
      >
        <label class="block text-sm font-semibold text-ink-900" for="chat-body">Kirim Message</label>
        <textarea
          id="chat-body"
          class="mt-3 min-h-28 w-full rounded-2xl border border-ink-100 bg-canvas-100 px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-brand-500"
          bind:value={body}
          maxlength="1000"
          placeholder="Tulis message untuk room global..."
        ></textarea>
        <div class="mt-3 flex items-center justify-between gap-3">
          <p class="text-xs text-ink-500">Maksimum 1000 karakter. Message tidak bisa diedit.</p>
          <Button variant="default" size="lg" type="submit" disabled={sending || body.trim() === ''}>
            {sending ? 'Sending...' : 'Send Message'}
          </Button>
        </div>
      </form>
    </div>

    <aside class="rounded-3xl border border-dashed border-ink-100 px-4 py-4 text-sm leading-6 text-ink-700">
      <p class="font-semibold text-ink-900">Connection Detail</p>
      <p class="mt-3">Connection ID: {$realtimeState.connection_id ?? '-'}</p>
      <p class="mt-2">Global channel ready: {channelReady ? 'ya' : 'belum'}</p>
      <p class="mt-2">
        Last heartbeat:
        {$realtimeState.last_heartbeat_at ?? 'belum ada'}
      </p>
      <p class="mt-2">Reconnect attempt: {$realtimeState.reconnect_attempt}</p>
      <p class="mt-2">Last error: {$realtimeState.last_error ?? '-'}</p>
      {#if canModerate}
        <div class="mt-4 rounded-2xl bg-canvas-100 px-4 py-3 text-xs text-ink-600">
          Role `dev` dapat menghapus message untuk moderasi room global.
        </div>
      {/if}
    </aside>
  </div>
</section>
