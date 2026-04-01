<script lang="ts">
  import { onMount } from 'svelte';
  import type { ChartConfiguration } from 'chart.js';

  import { authSession } from '$lib/auth/client';
  import {
    chartGridColor as resolveChartGridColor,
    chartTextColor as resolveChartTextColor,
  } from '$lib/chart-theme';
  import ChartCanvas from '$lib/components/app/chart-canvas.svelte';
  import DateRangeFilter from '$lib/components/app/date-range-filter.svelte';
  import EmptyState from '$lib/components/app/empty-state.svelte';
  import ExportActions from '$lib/components/app/export-actions.svelte';
  import GaugeRing from '$lib/components/app/gauge-ring.svelte';
  import MetricCard from '$lib/components/app/metric-card.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import PaginationControls from '$lib/components/app/pagination-controls.svelte';
  import Button from '$lib/components/ui/button/button.svelte';
  import { exportRowsToCSV, exportRowsToPDF, exportRowsToXLSX } from '$lib/exporters';
  import { formatDateTime, formatNumber } from '$lib/formatters';
  import {
    fetchNotifications,
    fetchUnreadNotificationCount,
    isNotificationEvent,
    markNotificationRead,
    notifyNotificationsChanged,
    resolveNotificationScope,
    type NotificationRecord,
    type NotificationScopeMode,
  } from '$lib/notifications/client';
  import { realtimeState } from '$lib/realtime/client';
  import { preferredStoreID } from '$lib/stores/preferences';
  import { resolvedTheme } from '$lib/theme';

  let notifications: NotificationRecord[] = [];
  let unreadCount = 0;
  let loading = true;
  let refreshing = false;
  let errorMessage = '';
  let markingID: string | null = null;
  let lastRealtimeKey: string | null = null;
  let lastScopeKey = '';
  let mounted = false;
  let platformScopeMode: NotificationScopeMode = 'auto';
  let searchTerm = '';
  let unreadFilter: 'all' | 'unread' | 'read' = 'all';
  let createdFrom = '';
  let createdTo = '';
  let page = 1;
  let pageSize = 10;
  let totalCount = 0;
  let lastQueryKey = '';

  $: role = $authSession?.user.role ?? '';
  $: canSelectPlatformScope = role === 'dev' || role === 'superadmin';
  $: scope = resolveNotificationScope(role, $preferredStoreID, platformScopeMode);
  $: channelReady = scope.channel !== '' && $realtimeState.channels.includes(scope.channel);
  $: usingRoleScope = scope.key.startsWith('role:');
  $: usingStoreScope =
    scope.key.startsWith('store:') ||
    (canSelectPlatformScope && platformScopeMode === 'store' && !scope.ready);
  $: unreadRatio = totalCount === 0 ? 0 : (unreadCount / totalCount) * 100;
  $: chartTextColor = resolveChartTextColor($resolvedTheme);
  $: chartGridColor = resolveChartGridColor($resolvedTheme);
  $: eventMix = buildEventMix(notifications);
  $: eventMixChart = buildEventMixChart(eventMix);

  onMount(() => {
    mounted = true;

    const unsubscribeRealtime = realtimeState.subscribe((snapshot) => {
      if (!mounted || !scope.ready) {
        return;
      }

      const latestEvent = snapshot.events[0];
      if (
        !latestEvent ||
        !isNotificationEvent(latestEvent.type) ||
        latestEvent.channel !== scope.channel
      ) {
        return;
      }

      const eventKey = `${latestEvent.created_at}:${latestEvent.channel}:${latestEvent.type}`;
      if (eventKey === lastRealtimeKey) {
        return;
      }

      lastRealtimeKey = eventKey;
      void loadNotifications(true);
    });

    return () => {
      mounted = false;
      unsubscribeRealtime();
    };
  });

  $: if (mounted) {
    const nextScopeKey = scope.key;
    if (nextScopeKey !== lastScopeKey) {
      lastScopeKey = nextScopeKey;
      page = 1;
      lastQueryKey = '';
      void loadNotifications();
    }
  }

  $: if (mounted && scope.ready) {
    const nextQueryKey = [scope.key, page, pageSize].join(':');

    if (nextQueryKey !== lastQueryKey) {
      lastQueryKey = nextQueryKey;
      void loadNotifications();
    }
  }

  async function loadNotifications(background = false) {
    if (!scope.ready) {
      notifications = [];
      totalCount = 0;
      unreadCount = 0;
      errorMessage = '';
      loading = false;
      refreshing = false;
      notifyNotificationsChanged();
      return;
    }

    if (background) {
      refreshing = true;
    } else {
      loading = true;
    }

    const [listResponse, unreadResponse] = await Promise.all([
      fetchNotifications({
        ...scope.params,
        query: searchTerm,
        readState: unreadFilter,
        createdFrom,
        createdTo,
        limit: pageSize,
        offset: (page - 1) * pageSize,
      }),
      fetchUnreadNotificationCount(scope.params),
    ]);

    if (!mounted) {
      return;
    }

    if (!listResponse.status || listResponse.message !== 'SUCCESS') {
      errorMessage = listResponse.message;
      loading = false;
      refreshing = false;
      return;
    }

    if (!unreadResponse.status || unreadResponse.message !== 'SUCCESS') {
      errorMessage = unreadResponse.message;
      loading = false;
      refreshing = false;
      return;
    }

    notifications = listResponse.data.items ?? [];
    totalCount = listResponse.data.total_count ?? 0;
    unreadCount = unreadResponse.data.unread_count ?? 0;
    lastQueryKey = [scope.key, page, pageSize].join(':');
    errorMessage = '';
    loading = false;
    refreshing = false;
    notifyNotificationsChanged();
  }

  async function handleMarkRead(notification: NotificationRecord) {
    if (notification.read_at !== null || markingID !== null) {
      return;
    }

    markingID = notification.id;
    errorMessage = '';

    const response = await markNotificationRead(notification.id, scope.params);
    markingID = null;

    if (!response.status || response.message !== 'MARKED_READ') {
      errorMessage = response.message;
      return;
    }

    const readAt = new Date().toISOString();
    notifications = notifications.map((item) =>
      item.id === notification.id ? { ...item, read_at: readAt } : item,
    );
    unreadCount = Math.max(0, unreadCount - 1);
    notifyNotificationsChanged();
  }

  function activatePlatformStoreScope() {
    platformScopeMode = 'store';
    page = 1;
  }

  function activatePlatformRoleScope() {
    platformScopeMode = 'role';
    page = 1;
  }

  function buildEventMix(records: NotificationRecord[]) {
    const counts = new Map<string, number>();

    for (const record of records) {
      const key = record.event_type.trim();
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }

    return Array.from(counts.entries())
      .map(([eventType, total]) => ({
        eventType,
        total,
      }))
      .sort((left, right) => right.total - left.total)
      .slice(0, 6);
  }

  function buildEventMixChart(
    mix: Array<{ eventType: string; total: number }>,
  ): ChartConfiguration<'bar'> {
    return {
      type: 'bar',
      data: {
        labels: mix.map((item) => compactEventLabel(item.eventType)),
        datasets: [
          {
            data: mix.map((item) => item.total),
            backgroundColor: ['#22c977', '#efc86d', '#0f7242', '#d66b5a', '#92826c', '#cfa74f'],
            borderRadius: 14,
            borderSkipped: false,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: {
            display: false,
          },
        },
        scales: {
          x: {
            ticks: {
              color: chartTextColor,
            },
            grid: {
              display: false,
            },
          },
          y: {
            ticks: {
              color: chartTextColor,
            },
            grid: {
              color: chartGridColor,
            },
          },
        },
      },
    };
  }

  function compactEventLabel(eventType: string) {
    return eventType.replace('.', '\n');
  }

  function eventTitle(eventType: string) {
    switch (eventType) {
      case 'member_payment.success':
        return 'Member payment success';
      case 'store_topup.success':
        return 'Store topup success';
      case 'withdraw.success':
        return 'Withdraw success';
      case 'withdraw.failed':
        return 'Withdraw failed';
      case 'callback.delivery_failed':
        return 'Callback delivery failed';
      case 'game.deposit.success':
        return 'Game deposit success';
      case 'game.withdraw.success':
        return 'Game withdraw success';
      case 'store.low_balance':
        return 'Low balance';
      default:
        return eventType;
    }
  }

  function exportNotificationsToCSV() {
    exportRowsToCSV(
      `${scope.label.replace(/\s+/g, '-').toLowerCase()}-notifications`,
      [
        { label: 'Event Type', value: (notification) => notification.event_type },
        { label: 'Title', value: (notification) => notification.title },
        { label: 'Body', value: (notification) => notification.body },
        { label: 'Scope Type', value: (notification) => notification.scope_type },
        { label: 'Scope ID', value: (notification) => notification.scope_id },
        {
          label: 'Read State',
          value: (notification) => (notification.read_at === null ? 'Unread' : 'Read')
        },
        { label: 'Created At', value: (notification) => formatDateTime(notification.created_at) },
        { label: 'Read At', value: (notification) => formatDateTime(notification.read_at) }
      ],
      notifications,
    );
  }

  function exportNotificationsToXLSX() {
    return exportRowsToXLSX(
      `${scope.label.replace(/\s+/g, '-').toLowerCase()}-notifications`,
      'Notifications',
      [
        { label: 'Event Type', value: (notification) => notification.event_type },
        { label: 'Title', value: (notification) => notification.title },
        { label: 'Body', value: (notification) => notification.body },
        { label: 'Scope Type', value: (notification) => notification.scope_type },
        { label: 'Scope ID', value: (notification) => notification.scope_id },
        {
          label: 'Read State',
          value: (notification) => (notification.read_at === null ? 'Unread' : 'Read')
        },
        { label: 'Created At', value: (notification) => formatDateTime(notification.created_at) },
        { label: 'Read At', value: (notification) => formatDateTime(notification.read_at) }
      ],
      notifications,
    );
  }

  function exportNotificationsToPDF() {
    return exportRowsToPDF(
      `${scope.label.replace(/\s+/g, '-').toLowerCase()}-notifications`,
      'Notification Feed',
      [
        { label: 'Event', value: (notification) => eventTitle(notification.event_type) },
        { label: 'Title', value: (notification) => notification.title },
        {
          label: 'State',
          value: (notification) => (notification.read_at === null ? 'Unread' : 'Read')
        },
        { label: 'Created', value: (notification) => formatDateTime(notification.created_at) }
      ],
      notifications,
    );
  }

  async function applyFilters() {
    page = 1;
    lastQueryKey = '';
    await loadNotifications();
  }

  async function resetFilters() {
    searchTerm = '';
    unreadFilter = 'all';
    createdFrom = '';
    createdTo = '';
    page = 1;
    lastQueryKey = '';
    await loadNotifications();
  }
</script>

<svelte:head>
  <title>Notifications | onixggr</title>
</svelte:head>

<section class="space-y-6">
  <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
    <div class="grid gap-6 xl:grid-cols-[1.12fr_0.88fr]">
      <div class="space-y-4">
        <div class="flex flex-wrap gap-3">
          <span class="status-chip">{scope.label}</span>
          <span class="status-chip">{channelReady ? 'channel ready' : 'channel pending'}</span>
          <span class="status-chip">{$realtimeState.status}</span>
        </div>
        <div class="space-y-3">
          <p class="section-kicker">Notification stream</p>
          <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
            Event feed yang sinkron antara database, unread state, dan websocket.
          </h1>
          <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
            Owner dan karyawan mengikuti store aktif. Role platform bisa pindah ke role stream atau
            store stream agar notifikasi operasional lintas tenant tetap terlihat jelas.
          </p>
        </div>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <MetricCard
          class="h-full"
          eyebrow="Unread"
          title="Unread items"
          value={formatNumber(unreadCount)}
          detail="Notifikasi yang belum ditandai read di scope aktif."
          tone="brand"
        />
        <MetricCard
          class="h-full"
          eyebrow="Feed"
          title="Total rows"
          value={formatNumber(totalCount)}
          detail="Jumlah row terfilter di scope aktif. Halaman ini membaca pagination backend."
          tone="accent"
        />
      </div>
    </div>

    {#if canSelectPlatformScope}
      <div class="mt-6 flex flex-wrap gap-3">
        <Button
          variant={usingRoleScope ? 'default' : 'outline'}
          size="sm"
          class={usingRoleScope ? 'text-ink-50' : ''}
          onclick={activatePlatformRoleScope}
        >
          Platform Stream
        </Button>
        <Button
          variant={usingStoreScope ? 'default' : 'outline'}
          size="sm"
          class={usingStoreScope ? 'text-ink-50' : ''}
          onclick={activatePlatformStoreScope}
        >
          Current Store Stream
        </Button>
      </div>
    {/if}
  </section>

  {#if errorMessage !== ''}
    <Notice tone="error" title="Feed Error" message={`Gagal memuat notification feed: ${errorMessage}`} />
  {/if}

  {#if !scope.ready}
    <EmptyState
      eyebrow="Scope Needed"
      title="Notification feed belum punya scope aktif"
      body={scope.description}
      actionHref={canSelectPlatformScope ? '/app/notifications' : ''}
      actionLabel=""
    />
  {:else if loading}
    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      {#each Array(4) as _}
        <article class="glass-panel animate-pulse rounded-[1.8rem] px-5 py-5" aria-hidden="true">
          <div class="h-3 w-24 rounded-full bg-canvas-100"></div>
          <div class="mt-4 h-8 w-32 rounded-full bg-canvas-100"></div>
          <div class="mt-3 h-3 w-full rounded-full bg-canvas-100"></div>
        </article>
      {/each}
    </div>
  {:else}
    <div class="grid gap-6 xl:grid-cols-[0.92fr_1.08fr]">
      <section class="space-y-6">
        <div class="grid gap-4 sm:grid-cols-2">
          <GaugeRing
            label="Unread ratio"
            value={unreadRatio}
            detail="Proporsi unread global terhadap total row terfilter di scope aktif."
            tone={unreadRatio >= 40 ? 'accent' : 'brand'}
          />
          <article class="glass-panel rounded-[2rem] p-5">
            <p class="section-kicker !text-brand-700">Feed status</p>
            <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
              Scope telemetry
            </h2>
            <div class="mt-5 space-y-3 text-sm leading-6 text-ink-700">
              <p>Label: <span class="font-semibold text-ink-900">{scope.label}</span></p>
              <p>Status WS: <span class="font-semibold text-ink-900">{$realtimeState.status}</span></p>
              <p>Channel: <span class="font-mono text-xs text-ink-900">{scope.channel || '-'}</span></p>
              <p>Unread: <span class="font-semibold text-ink-900">{formatNumber(unreadCount)}</span></p>
            </div>
          </article>
        </div>

        <div class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Event mix</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Most frequent notification types
              </h2>
            </div>
            <span class="surface-chip">{formatNumber(eventMix.length)} type</span>
          </div>

          {#if eventMix.length === 0}
            <div class="mt-6">
              <EmptyState
                eyebrow="Event Mix"
                title="Belum ada event yang bisa divisualkan"
                body="Saat notification mulai masuk, halaman ini akan memetakan distribusi event type otomatis."
              />
            </div>
          {:else}
            <div class="mt-6">
              <ChartCanvas class="h-[320px]" config={eventMixChart} />
            </div>
          {/if}
        </div>
      </section>

      <section class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
        <div class="flex items-end justify-between gap-4">
          <div>
            <p class="section-kicker !text-brand-700">Timeline</p>
            <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
              Notification feed
            </h2>
          </div>
          {#if refreshing}
            <span class="surface-chip">refreshing</span>
          {/if}
        </div>

        <div class="mt-6 grid gap-4 xl:grid-cols-[12rem_minmax(0,1fr)]">
          <label class="space-y-2">
            <span class="text-sm font-medium text-ink-700">Read state</span>
            <select
              bind:value={unreadFilter}
              class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
            >
              <option value="all">Semua state</option>
              <option value="unread">Unread</option>
              <option value="read">Read</option>
            </select>
          </label>

          <label class="space-y-2">
            <span class="text-sm font-medium text-ink-700">Cari event</span>
            <input
              bind:value={searchTerm}
              class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
              placeholder="Cari title, body, atau event type"
            />
          </label>
        </div>

        <div class="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
          <DateRangeFilter bind:start={createdFrom} bind:end={createdTo} label="Created at" />
          <ExportActions
            count={notifications.length}
            disabled={notifications.length === 0}
            onCsv={exportNotificationsToCSV}
            onXlsx={exportNotificationsToXLSX}
            onPdf={exportNotificationsToPDF}
          />
        </div>

        <div class="mt-4 flex flex-wrap gap-3">
          <Button variant="brand" size="sm" onclick={applyFilters} disabled={refreshing}>
            Apply filters
          </Button>
          <Button variant="outline" size="sm" onclick={resetFilters} disabled={refreshing}>
            Reset
          </Button>
        </div>

        {#if totalCount === 0}
          <div class="mt-6">
            <EmptyState
              eyebrow="Notification Feed"
              title="Belum ada notifikasi di scope ini"
              body="Saat event transaksi, withdraw, callback, atau low balance masuk, feed ini akan terisi otomatis dan ikut bergerak lewat WebSocket."
            />
          </div>
        {:else}
          <div class="mt-6 space-y-3">
            {#each notifications as notification}
              <article
                class={`rounded-[1.6rem] border px-4 py-4 shadow-[0_16px_34px_rgba(7,16,12,0.08)] transition ${
                  notification.read_at === null
                    ? 'border-brand-200 bg-linear-to-r from-brand-100/55 to-white'
                    : 'border-ink-100 bg-white/78'
                }`}
              >
                <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                  <div class="space-y-3">
                    <div class="flex flex-wrap items-center gap-2">
                      <p class="text-sm font-semibold text-ink-900">{notification.title}</p>
                      <span class="surface-chip">
                        {notification.read_at === null ? 'unread' : 'read'}
                      </span>
                    </div>

                    <p class="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-brand-700">
                      {eventTitle(notification.event_type)}
                    </p>

                    <p class="max-w-3xl text-sm leading-7 text-ink-700">{notification.body}</p>
                  </div>

                  <div class="flex items-start gap-3 lg:flex-col lg:items-end">
                    <p class="text-xs text-ink-500">{formatDateTime(notification.created_at)}</p>
                    {#if notification.read_at === null}
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={markingID === notification.id}
                        onclick={() => handleMarkRead(notification)}
                      >
                        {markingID === notification.id ? 'Marking...' : 'Mark Read'}
                      </Button>
                    {:else}
                      <p class="text-xs text-ink-500">
                        Read {notification.read_at ? formatDateTime(notification.read_at) : ''}
                      </p>
                    {/if}
                  </div>
                </div>
              </article>
            {/each}
          </div>

          <div class="mt-5">
            <PaginationControls bind:page bind:pageSize totalItems={totalCount} />
          </div>
        {/if}
      </section>
    </div>
  {/if}
</section>
