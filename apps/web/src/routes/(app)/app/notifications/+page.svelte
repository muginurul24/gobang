<svelte:head>
  <title>Notifications | onixggr</title>
</svelte:head>

<script lang="ts">
  import { onMount } from 'svelte';

  import { authSession } from '$lib/auth/client';
  import EmptyState from '$lib/components/app/empty-state.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import Button from '$lib/components/ui/button/button.svelte';
  import {
    fetchNotifications,
    fetchUnreadNotificationCount,
    isNotificationEvent,
    markNotificationRead,
    notifyNotificationsChanged,
    resolveNotificationScope,
    type NotificationRecord,
    type NotificationScopeMode
  } from '$lib/notifications/client';
  import { realtimeState } from '$lib/realtime/client';
  import { preferredStoreID } from '$lib/stores/preferences';

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

  $: role = $authSession?.user.role ?? '';
  $: canSelectPlatformScope = role === 'dev' || role === 'superadmin';
  $: scope = resolveNotificationScope(role, $preferredStoreID, platformScopeMode);
  $: channelReady =
    scope.channel !== '' && $realtimeState.channels.includes(scope.channel);
  $: usingRoleScope = scope.key.startsWith('role:');
  $: usingStoreScope =
    scope.key.startsWith('store:') ||
    (canSelectPlatformScope && platformScopeMode === 'store' && !scope.ready);

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
      void loadNotifications();
    }
  }

  async function loadNotifications(background = false) {
    if (!scope.ready) {
      notifications = [];
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
        limit: 50
      }),
      fetchUnreadNotificationCount(scope.params)
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

    notifications = listResponse.data ?? [];
    unreadCount = unreadResponse.data.unread_count ?? 0;
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

  function formatTimestamp(value: string) {
    return new Intl.DateTimeFormat('id-ID', {
      day: '2-digit',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit'
    }).format(new Date(value));
  }

  function activatePlatformStoreScope() {
    platformScopeMode = 'store';
  }

  function activatePlatformRoleScope() {
    platformScopeMode = 'role';
  }
</script>

<section class="space-y-6">
  <div class="glass-panel rounded-4xl px-6 py-7">
    <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
          Notification Stream
        </p>
        <h2 class="mt-3 font-display text-4xl font-bold tracking-tight text-ink-900">
          Feed realtime yang dibackup database, unread state, dan scope dashboard.
        </h2>
        <p class="mt-3 max-w-3xl text-sm leading-7 text-ink-700">
          Halaman ini memakai endpoint `notifications` yang sama dengan stream WebSocket. Owner
          dan karyawan mengikuti store aktif, sementara role platform bisa pindah antara scope toko
          aktif dan scope role.
        </p>
      </div>

      <div class="rounded-3xl border border-ink-100 px-5 py-4 text-sm text-ink-700 lg:w-80">
        <p class="font-semibold text-ink-900">Feed Status</p>
        <p class="mt-2 uppercase tracking-[0.18em] text-brand-700">{scope.label}</p>
        <p class="mt-2 text-xs leading-5 text-ink-500">{scope.description}</p>
        <p class="mt-3 text-sm text-ink-800">{unreadCount} unread</p>
        <p class="mt-1 text-xs text-ink-500">
          Realtime channel {channelReady ? 'siap' : 'belum siap'} · {$realtimeState.status}
        </p>
      </div>
    </div>

    {#if canSelectPlatformScope}
      <div class="mt-6 flex flex-wrap gap-3">
        <Button
          variant={usingRoleScope ? 'default' : 'outline'}
          size="sm"
          onclick={activatePlatformRoleScope}
        >
          Platform Stream
        </Button>
        <Button
          variant={usingStoreScope ? 'default' : 'outline'}
          size="sm"
          onclick={activatePlatformStoreScope}
        >
          Current Store Stream
        </Button>
      </div>
    {/if}
  </div>

  {#if errorMessage !== ''}
    <Notice tone="error" message={`Gagal memuat notification feed: ${errorMessage}`} />
  {/if}

  {#if !scope.ready}
    <article class="rounded-3xl border border-dashed border-ink-200 bg-canvas-50 px-5 py-6 text-sm text-ink-700">
      <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">Scope Needed</p>
      <h3 class="mt-3 font-display text-2xl font-bold tracking-tight text-ink-900">
        Notification feed belum punya scope aktif.
      </h3>
      <p class="mt-3 max-w-2xl leading-6">{scope.description}</p>
      {#if canSelectPlatformScope}
        <div class="mt-5">
          <Button variant="default" size="sm" onclick={activatePlatformRoleScope}>
            Pindah ke Platform Stream
          </Button>
        </div>
      {/if}
    </article>
  {:else if loading}
    <div class="space-y-3">
      {#each Array(4) as _}
        <article class="glass-panel animate-pulse rounded-3xl px-5 py-5" aria-hidden="true">
          <div class="h-3 w-24 rounded-full bg-canvas-100"></div>
          <div class="mt-4 h-4 w-2/3 rounded-full bg-canvas-100"></div>
          <div class="mt-3 h-3 w-full rounded-full bg-canvas-100"></div>
          <div class="mt-2 h-3 w-5/6 rounded-full bg-canvas-100"></div>
        </article>
      {/each}
    </div>
  {:else if notifications.length === 0}
    <EmptyState
      eyebrow="Notification Feed"
      title="Belum ada notifikasi di scope ini"
      body="Saat event transaksi, withdraw, callback, atau low balance masuk, feed ini akan terisi otomatis dan ikut bergerak lewat WebSocket."
    />
  {:else}
    <div class="space-y-3">
      {#if refreshing}
        <Notice tone="info" message="Feed sedang menyegarkan event terbaru dari WebSocket..." />
      {/if}

      {#each notifications as notification}
        <article
          class={`glass-panel rounded-3xl px-5 py-5 ${
            notification.read_at === null
              ? 'border border-accent-200 bg-accent-50/50'
              : 'border border-ink-100'
          }`}
        >
          <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <div class="flex flex-wrap items-center gap-2">
                <p class="font-semibold text-ink-900">{notification.title}</p>
                <span
                  class={`rounded-full px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.18em] ${
                    notification.read_at === null
                      ? 'bg-accent-100 text-accent-800'
                      : 'bg-canvas-100 text-ink-500'
                  }`}
                >
                  {notification.read_at === null ? 'Unread' : 'Read'}
                </span>
              </div>
              <p class="mt-2 text-xs uppercase tracking-[0.18em] text-ink-500">
                {notification.event_type}
              </p>
              <p class="mt-3 max-w-3xl text-sm leading-6 text-ink-700">{notification.body}</p>
            </div>

            <div class="flex items-start gap-3 lg:flex-col lg:items-end">
              <p class="text-xs text-ink-500">{formatTimestamp(notification.created_at)}</p>
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
                  Read {notification.read_at ? formatTimestamp(notification.read_at) : ''}
                </p>
              {/if}
            </div>
          </div>
        </article>
      {/each}
    </div>
  {/if}
</section>
