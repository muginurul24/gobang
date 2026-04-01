<svelte:head>
  <title>Chat | onixggr</title>
</svelte:head>

<script lang="ts">
  import { onMount } from 'svelte';

  import { authSession } from '$lib/auth/client';
  import DateRangeFilter from '$lib/components/app/date-range-filter.svelte';
  import EmptyState from '$lib/components/app/empty-state.svelte';
  import ExportActions from '$lib/components/app/export-actions.svelte';
  import MetricCard from '$lib/components/app/metric-card.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import PaginationControls from '$lib/components/app/pagination-controls.svelte';
  import type { ChatMessage } from '$lib/chat/client';
  import { deleteChatMessage, fetchChatMessages, sendChatMessage } from '$lib/chat/client';
  import Button from '$lib/components/ui/button/button.svelte';
  import { exportRowsToCSV, exportRowsToPDF, exportRowsToXLSX } from '$lib/exporters';
  import { formatDateTime, formatNumber } from '$lib/formatters';
  import { realtimeState } from '$lib/realtime/client';

  let messages: ChatMessage[] = [];
  let totalMessageCount = 0;
  let body = '';
  let loading = true;
  let refreshing = false;
  let sending = false;
  let deletingID: string | null = null;
  let errorMessage: string | null = null;
  let lastRealtimeKey: string | null = null;
  let lastQueryKey = '';
  let searchTerm = '';
  let roleFilter = 'all';
  let createdFrom = '';
  let createdTo = '';
  let page = 1;
  let pageSize = 12;

  $: role = $authSession?.user.role ?? '';
  $: canModerate = role === 'dev';
  $: channelReady = $realtimeState.channels.includes('global_chat');
  $: recentMessageCount = messages.filter((message) => {
    const createdAt = new Date(message.created_at).getTime();
    return Number.isFinite(createdAt) && Date.now() - createdAt <= 24 * 60 * 60 * 1000;
  }).length;
  $: roleMix = buildRoleMix(messages);

  onMount(() => {
    let active = true;

    async function loadMessages(background = false) {
      if (background) {
        refreshing = true;
      }

      const response = await fetchChatMessages({
        query: searchTerm,
        role: roleFilter,
        createdFrom,
        createdTo,
        limit: pageSize,
        offset: (page - 1) * pageSize
      });
      if (!active) {
        return;
      }

      if (!response.status || response.message !== 'SUCCESS') {
        errorMessage = response.message;
        loading = false;
        refreshing = false;
        return;
      }

      messages = response.data.items ?? [];
      totalMessageCount = response.data.total_count ?? 0;
      lastQueryKey = `${page}:${pageSize}`;
      loading = false;
      refreshing = false;
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

      void loadMessages(true);
    });

    void loadMessages();

    return () => {
      active = false;
      unsubscribe();
    };
  });

  $: {
    const nextQueryKey = `${page}:${pageSize}`;
    if (!loading && nextQueryKey !== lastQueryKey) {
      lastQueryKey = nextQueryKey;
      void refreshMessages();
    }
  }

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
    page = 1;
    lastQueryKey = '';
    await refreshMessages();
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
      return;
    }

    await refreshMessages(true);
  }

  function formatTimestamp(value: string) {
    return formatDateTime(value);
  }

  function buildRoleMix(items: ChatMessage[]) {
    const counts = new Map<string, number>();

    for (const item of items) {
      counts.set(item.sender_role, (counts.get(item.sender_role) ?? 0) + 1);
    }

    return Array.from(counts.entries()).sort((left, right) => right[1] - left[1]);
  }

  function exportMessagesToCSV() {
    exportRowsToCSV(
      'global-chat',
      [
        { label: 'Sender Username', value: (message) => message.sender_username },
        { label: 'Sender Role', value: (message) => message.sender_role },
        { label: 'Body', value: (message) => message.body },
        { label: 'Created At', value: (message) => formatDateTime(message.created_at) }
      ],
      messages,
    );
  }

  function exportMessagesToXLSX() {
    return exportRowsToXLSX(
      'global-chat',
      'Chat',
      [
        { label: 'Sender Username', value: (message) => message.sender_username },
        { label: 'Sender Role', value: (message) => message.sender_role },
        { label: 'Body', value: (message) => message.body },
        { label: 'Created At', value: (message) => formatDateTime(message.created_at) }
      ],
      messages,
    );
  }

  function exportMessagesToPDF() {
    return exportRowsToPDF(
      'global-chat',
      'Global Chat Room',
      [
        { label: 'Sender', value: (message) => message.sender_username },
        { label: 'Role', value: (message) => message.sender_role },
        { label: 'Created', value: (message) => formatDateTime(message.created_at) },
        { label: 'Body', value: (message) => message.body }
      ],
      messages,
    );
  }

  async function refreshMessages(background = false) {
    if (background) {
      refreshing = true;
    } else {
      loading = true;
    }

    const response = await fetchChatMessages({
      query: searchTerm,
      role: roleFilter,
      createdFrom,
      createdTo,
      limit: pageSize,
      offset: (page - 1) * pageSize
    });

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = response.message;
      loading = false;
      refreshing = false;
      return;
    }

    messages = response.data.items ?? [];
    totalMessageCount = response.data.total_count ?? 0;
    lastQueryKey = `${page}:${pageSize}`;
    errorMessage = null;
    loading = false;
    refreshing = false;
  }

  async function applyFilters() {
    page = 1;
    lastQueryKey = '';
    await refreshMessages();
  }

  async function resetFilters() {
    searchTerm = '';
    roleFilter = 'all';
    createdFrom = '';
    createdTo = '';
    page = 1;
    lastQueryKey = '';
    await refreshMessages();
  }
</script>

<section class="space-y-6">
  <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
    <div class="grid gap-6 2xl:grid-cols-[1.08fr_0.92fr]">
      <div class="space-y-4">
        <div class="flex flex-wrap gap-3">
          <span class="status-chip">global_chat</span>
          <span class="status-chip">{$realtimeState.status}</span>
          <span class="status-chip">{canModerate ? 'dev moderation' : 'participant mode'}</span>
        </div>
        <div class="space-y-3">
          <p class="section-kicker">Ops room</p>
          <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
            Satu room global untuk koordinasi operasional real-time.
          </h1>
          <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
            Chat ini memang sengaja sederhana: satu room, tidak ada edit, tidak ada DM, history
            dipruning 7 hari, dan dev dapat melakukan moderasi delete.
          </p>
        </div>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <MetricCard
          class="h-full"
          eyebrow="Room Volume"
          title="Filtered rows"
          value={formatNumber(totalMessageCount)}
          detail="Total message terfilter yang tersedia dari backend."
          tone="brand"
        />
        <MetricCard
          class="h-full"
          eyebrow="24 Hours"
          title="Recent activity"
          value={formatNumber(recentMessageCount)}
          detail="Message yang masuk dalam 24 jam terakhir."
          tone="accent"
        />
      </div>
    </div>
  </section>

  <section class="glass-panel overflow-hidden rounded-[2.3rem]">
    <div class="grid gap-4 p-6 2xl:grid-cols-[minmax(0,1fr)_18rem]">
      <div class="space-y-3">
        <article class="rounded-[1.8rem] bg-canvas-100 px-5 py-4 text-sm leading-6 text-ink-700">
          Chat ini hanya punya satu room: `global_chat`. Tidak ada DM, tidak ada edit, dan history
          dibersihkan otomatis setelah 7 hari.
        </article>
        <article class="surface-dark rounded-[1.8rem] px-5 py-4 text-sm leading-6 text-white">
          Pengiriman message lewat HTTP, distribusi realtime lewat WebSocket `global_chat`. Saat dev
          menghapus message, item akan hilang untuk semua peserta.
        </article>

        {#if errorMessage}
          <Notice tone="error" title="Chat Error" message={errorMessage} />
        {/if}

        <article class="rounded-[1.9rem] border border-ink-100 px-5 py-4 text-sm leading-6 text-ink-700">
        <div class="flex items-center justify-between gap-4">
          <div>
            <p class="font-semibold text-ink-900">Room Timeline</p>
            <p class="mt-1 text-xs text-ink-500">
              {formatNumber(totalMessageCount)} message dalam hasil filter aktif
            </p>
          </div>
          <p class="text-xs uppercase tracking-[0.18em] text-brand-700">
            {$realtimeState.status}
          </p>
        </div>

        <div class="mt-4 space-y-3">
          <div class="grid gap-4 2xl:grid-cols-[12rem_minmax(0,1fr)]">
            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Role</span>
              <select
                bind:value={roleFilter}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
              >
                <option value="all">Semua role</option>
                {#each roleMix as [senderRole]}
                  <option value={senderRole}>{senderRole}</option>
                {/each}
              </select>
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Cari message</span>
              <input
                bind:value={searchTerm}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                placeholder="Cari sender, role, atau isi message"
              />
            </label>
          </div>

          <div class="grid gap-4 2xl:grid-cols-[minmax(0,1fr)_minmax(18rem,24rem)]">
            <DateRangeFilter bind:start={createdFrom} bind:end={createdTo} label="Created at" />
            <ExportActions
              count={messages.length}
              disabled={messages.length === 0}
              onCsv={exportMessagesToCSV}
              onXlsx={exportMessagesToXLSX}
              onPdf={exportMessagesToPDF}
            />
          </div>

          <div class="flex flex-wrap gap-3">
            <Button variant="brand" size="sm" onclick={applyFilters} disabled={refreshing}>
              Apply filters
            </Button>
            <Button variant="outline" size="sm" onclick={resetFilters} disabled={refreshing}>
              Reset
            </Button>
          </div>

          <div class="max-h-[32rem] space-y-3 overflow-y-auto pr-1 soft-scroll">
            {#if loading}
              <p class="rounded-[1.4rem] bg-canvas-100 px-4 py-3 text-sm text-ink-600">
                Memuat room global...
              </p>
            {:else if totalMessageCount === 0}
              <EmptyState
                eyebrow="Global Chat"
                title="Belum ada message"
                body="Belum ada message di room global. Kirim message pertama untuk memulai percakapan operasional."
              />
            {:else}
              {#each messages as message}
                <div class="rounded-[1.5rem] bg-canvas-100 px-4 py-4">
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
        </div>

        {#if totalMessageCount > 0}
          <div class="mt-4">
            <PaginationControls bind:page bind:pageSize totalItems={totalMessageCount} />
          </div>
        {/if}
      </article>

        <form
          class="rounded-[1.9rem] border border-ink-100 px-5 py-4"
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

      <aside class="rounded-[1.9rem] border border-dashed border-ink-100 px-4 py-4 text-sm leading-6 text-ink-700">
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
          <div class="mt-4 rounded-[1.4rem] bg-canvas-100 px-4 py-3 text-xs text-ink-600">
            Role `dev` dapat menghapus message untuk moderasi room global.
          </div>
        {/if}
      </aside>
    </div>
  </section>
</section>
