<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { onMount } from 'svelte';

  import Button from '$lib/components/ui/button/button.svelte';
  import PaginationControls from '$lib/components/app/pagination-controls.svelte';
  import ThemeToggle from '$lib/components/app/theme-toggle.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import {
    authSession,
    initializeAuthSession,
    logoutCurrentSession,
    syncProfile,
  } from '$lib/auth/client';
  import { formatCurrency, formatNumber } from '$lib/formatters';
  import { fetchUnreadNotificationCount, isNotificationEvent, resolveNotificationScope, subscribeNotificationsChanged } from '$lib/notifications/client';
  import { connectRealtime, disconnectRealtime, realtimeState } from '$lib/realtime/client';
  import { fetchStore, fetchStoreDirectory, isStoreLowBalance, type Store, type StoreDirectorySummary } from '$lib/stores/client';
  import { hydratePreferredStoreID, pickPreferredStoreID, preferredStoreID, setPreferredStoreID } from '$lib/stores/preferences';

  let ready = false;
  let storeDirectoryLoading = true;
  let storeDirectoryError = '';
  let accessibleStores: Store[] = [];
  let storeDirectorySummary: StoreDirectorySummary = {
    total_count: 0,
    active_count: 0,
    inactive_count: 0,
    banned_count: 0,
    deleted_count: 0,
    low_balance_count: 0
  };
  let storeQuery = '';
  let storePage = 1;
  let storePageSize = 12;
  let selectedStoreID = '';
  let selectedStoreSummary: Store | null = null;
  let unreadNotificationCount = 0;
  let notificationLoading = false;
  let lastNotificationEventKey: string | null = null;
  let lastNotificationScopeKey = '';
  let lastStoreDirectoryKey = '';
  let viewActive = false;

  $: role = $authSession?.user.role ?? '';
  $: currentStore =
    accessibleStores.find((store) => store.id === selectedStoreID) ??
    (selectedStoreSummary?.id === selectedStoreID ? selectedStoreSummary : null);
  $: visibleLowBalanceStores = accessibleStores.filter((store) => isStoreLowBalance(store));
  $: selectedStoreIsLowBalance = currentStore ? isStoreLowBalance(currentStore) : false;
  $: notificationScope = resolveNotificationScope(role, selectedStoreID);
  $: notificationBadge = unreadNotificationCount > 99
    ? '99+'
    : unreadNotificationCount > 0
      ? String(unreadNotificationCount)
      : '';
  $: nav = [
    { href: '/app', label: 'Dashboard', description: 'realtime cards' },
    { href: '/app/notifications', label: 'Notifications', description: 'event stream', badge: notificationBadge },
    ...(
      role === 'dev' || role === 'superadmin'
        ? [{ href: '/app/ops', label: 'Ops', description: 'health + callbacks' }]
        : []
    ),
    { href: '/app/stores', label: 'Stores', description: 'token + callback' },
    { href: '/app/api-docs', label: 'API Docs', description: 'owner integration' },
    { href: '/app/catalog', label: 'Catalog', description: 'provider + games' },
    { href: '/app/members', label: 'Members', description: 'store identities' },
    ...(
      role === 'karyawan'
        ? []
        : [
            { href: '/app/topups', label: 'Topups', description: 'qris store credit' },
            { href: '/app/bank-accounts', label: 'Banking', description: 'withdraw accounts' },
            { href: '/app/withdrawals', label: 'Withdrawals', description: 'payout desk' },
            { href: '/app/audit', label: 'Audit', description: 'activity trail' },
          ]
    ),
    { href: '/app/security', label: 'Security', description: '2fa + allowlist' },
    { href: '/app/chat', label: 'Global Chat', description: 'ops room' },
    { href: '/', label: 'Public', description: 'marketing shell' },
  ];

  async function loadAccessibleStores() {
    storeDirectoryLoading = true;
    storeDirectoryError = '';

    const response = await fetchStoreDirectory({
      query: storeQuery,
      limit: storePageSize,
      offset: (storePage - 1) * storePageSize
    });
    if (!viewActive) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      storeDirectoryError =
        response.message === 'FORBIDDEN'
          ? 'Store switch tidak tersedia untuk role ini.'
          : 'Store switch belum bisa dimuat. Halaman lain tetap bisa dipakai.';
      accessibleStores = [];
      storeDirectorySummary = {
        total_count: 0,
        active_count: 0,
        inactive_count: 0,
        banned_count: 0,
        deleted_count: 0,
        low_balance_count: 0
      };
      selectedStoreSummary = null;
      selectedStoreID = '';
      storeDirectoryLoading = false;
      return;
    }

    accessibleStores = response.data.items ?? [];
    storeDirectorySummary = response.data.summary ?? storeDirectorySummary;
    selectedStoreID = pickPreferredStoreID(accessibleStores, selectedStoreID);
    setPreferredStoreID(selectedStoreID);
    if (selectedStoreID !== '') {
      await syncSelectedStoreSummary(selectedStoreID);
    } else {
      selectedStoreSummary = null;
    }
    storeDirectoryLoading = false;
  }

  onMount(() => {
    viewActive = true;
    hydratePreferredStoreID();

    const unsubscribeStorePreference = preferredStoreID.subscribe((storeID) => {
      if (!viewActive) {
        return;
      }

      if (storeID !== '' && accessibleStores.some((store) => store.id === storeID)) {
        selectedStoreID = storeID;
      }
    });

    async function loadUnreadNotifications() {
      if (!viewActive) {
        return;
      }

      if (!notificationScope.ready) {
        unreadNotificationCount = 0;
        notificationLoading = false;
        return;
      }

      notificationLoading = true;
      const response = await fetchUnreadNotificationCount(notificationScope.params);
      if (!viewActive) {
        return;
      }

      notificationLoading = false;
      if (!response.status || response.message !== 'SUCCESS') {
        return;
      }

      unreadNotificationCount = response.data.unread_count ?? 0;
    }

    void (async () => {
      await initializeAuthSession();

      if (!$authSession) {
        disconnectRealtime();
        await goto('/login');
        return;
      }

      const profile = await syncProfile();
      if (!viewActive) {
        return;
      }

      if (!profile.status || profile.message !== 'SUCCESS') {
        disconnectRealtime();
        await goto('/login');
        return;
      }

      connectRealtime();
      await loadAccessibleStores();
      ready = true;
    })();

    const unsubscribeRealtime = realtimeState.subscribe((snapshot) => {
      if (!viewActive || !notificationScope.ready) {
        return;
      }

      const latestEvent = snapshot.events[0];
      if (
        !latestEvent ||
        !isNotificationEvent(latestEvent.type) ||
        latestEvent.channel !== notificationScope.channel
      ) {
        return;
      }

      const eventKey = `${latestEvent.created_at}:${latestEvent.channel}:${latestEvent.type}`;
      if (eventKey === lastNotificationEventKey) {
        return;
      }

      lastNotificationEventKey = eventKey;
      void loadUnreadNotifications();
    });

    const unsubscribeNotificationsChanged = subscribeNotificationsChanged(() => {
      void loadUnreadNotifications();
    });

    return () => {
      viewActive = false;
      unsubscribeStorePreference();
      unsubscribeRealtime();
      unsubscribeNotificationsChanged();
      disconnectRealtime();
    };
  });

  $: if (ready) {
    const nextStoreDirectoryKey = `${storeQuery}:${storePage}:${storePageSize}`;
    if (nextStoreDirectoryKey !== lastStoreDirectoryKey) {
      lastStoreDirectoryKey = nextStoreDirectoryKey;
      void loadAccessibleStores();
    }
  }

  $: if (ready) {
    const nextScopeKey = notificationScope.key;
    if (nextScopeKey !== lastNotificationScopeKey) {
      lastNotificationScopeKey = nextScopeKey;
      if (!notificationScope.ready) {
        unreadNotificationCount = 0;
        notificationLoading = false;
      } else {
        notificationLoading = true;
        void fetchUnreadNotificationCount(notificationScope.params).then((response) => {
          notificationLoading = false;
          if (!response.status || response.message !== 'SUCCESS') {
            return;
          }

          unreadNotificationCount = response.data.unread_count ?? 0;
        });
      }
    }
  }

  async function signOut() {
    disconnectRealtime();
    await logoutCurrentSession();
    await goto('/login');
  }

  function switchStore(storeID: string) {
    selectedStoreID = storeID;
    setPreferredStoreID(storeID);
    void syncSelectedStoreSummary(storeID);
  }

  async function syncSelectedStoreSummary(storeID: string) {
    const matched = accessibleStores.find((store) => store.id === storeID) ?? null;
    if (matched) {
      selectedStoreSummary = matched;
      return;
    }

    if (storeID === '') {
      selectedStoreSummary = null;
      return;
    }

    const response = await fetchStore(storeID);
    if (!response.status || response.message !== 'SUCCESS') {
      selectedStoreSummary = null;
      return;
    }

    selectedStoreSummary = response.data ?? null;
  }

  async function applyStoreDirectorySearch() {
    storePage = 1;
    lastStoreDirectoryKey = '';
    await loadAccessibleStores();
  }

  async function resetStoreDirectorySearch() {
    storeQuery = '';
    storePage = 1;
    lastStoreDirectoryKey = '';
    await loadAccessibleStores();
  }

  function isActive(href: string) {
    const pathname = $page.url.pathname;
    return href === '/app' ? pathname === '/app' : pathname.startsWith(href);
  }

  function realtimeLabel() {
    switch ($realtimeState.status) {
      case 'connected':
        return 'live';
      case 'reconnecting':
        return 'retrying';
      case 'connecting':
        return 'connecting';
      case 'error':
        return 'error';
      default:
        return 'idle';
    }
  }

  function pageSummary() {
    if (role === 'karyawan') {
      return 'Store-scoped command center untuk monitoring, members, security, dan realtime feed.';
    }

    if (role === 'owner') {
      return 'Command center owner untuk toko, QRIS, withdraw, callback, dan integrasi store API.';
    }

    return 'Platform command surface untuk monitoring lintas store, audit, realtime, dan observability.';
  }
</script>

{#if ready}
  <div class="matrix-page" data-app-shell="ready">
    <div class="shell-width mx-auto flex min-h-screen flex-col gap-6 pb-10 pt-4 sm:pt-6">
      <section class="surface-dark surface-grid overflow-hidden rounded-[2.6rem] px-5 py-5 text-white sm:px-7 sm:py-6">
        <div class="grid gap-6 xl:grid-cols-[1.15fr_0.85fr]">
          <div class="space-y-5">
            <div class="flex flex-wrap items-center gap-3">
              <span class="status-chip">role {role || 'guest'}</span>
              <span class="status-chip">realtime {realtimeLabel()}</span>
              {#if notificationBadge !== ''}
                <span class="status-chip">{notificationBadge} unread</span>
              {/if}
            </div>

            <div class="space-y-3">
              <p class="section-kicker">Command Matrix</p>
              <h1 class="font-display text-4xl font-bold tracking-tight text-white sm:text-5xl">
                Operasional toko, transaksi, dan integrasi dalam satu shell.
              </h1>
              <p class="max-w-3xl text-sm leading-7 text-white/70 sm:text-base">
                {pageSummary()}
              </p>
            </div>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <article class="rounded-[1.8rem] border border-white/12 bg-white/7 p-5 backdrop-blur">
              <p class="text-[0.68rem] font-semibold uppercase tracking-[0.28em] text-white/45">
                Session
              </p>
              <p class="mt-3 text-lg font-semibold text-white">
                {$authSession?.user.username ?? '-'}
              </p>
              <p class="mt-1 text-sm text-white/62">{$authSession?.user.role ?? '-'}</p>
              <p class="mt-4 text-xs leading-6 text-white/52">
                {$realtimeState.channels.length} realtime channel aktif di sesi ini.
              </p>
            </article>

            <article class="rounded-[1.8rem] border border-white/12 bg-white/7 p-5 backdrop-blur">
              <p class="text-[0.68rem] font-semibold uppercase tracking-[0.28em] text-white/45">
                Active Store
              </p>
              {#if currentStore}
                <p class="mt-3 text-lg font-semibold text-white">{currentStore.name}</p>
                <p class="mt-1 text-sm text-white/62">{currentStore.slug}</p>
                <p class="mt-4 text-xs leading-6 text-white/52">
                  Balance {formatCurrency(currentStore.current_balance)}{#if currentStore.low_balance_threshold}
                    {' '}· threshold {formatCurrency(currentStore.low_balance_threshold)}
                  {/if}
                </p>
              {:else if storeDirectoryLoading}
                <p class="mt-3 text-sm text-white/60">Memuat store directory...</p>
              {:else}
                <p class="mt-3 text-sm text-white/60">Belum ada store aktif untuk sesi ini.</p>
              {/if}
            </article>
          </div>
        </div>

        <div class="mt-6 overflow-x-auto soft-scroll">
          <nav class="flex min-w-max gap-3 pb-1">
            {#each nav as item}
              <a
                aria-current={isActive(item.href) ? 'page' : undefined}
                class="app-nav-link"
                data-active={isActive(item.href)}
                href={item.href}
              >
                <span>{item.label}</span>
                {#if item.badge}
                  <span class="rounded-full bg-white/10 px-2 py-1 text-[0.68rem] font-semibold">
                    {item.badge}
                  </span>
                {/if}
              </a>
            {/each}
          </nav>
        </div>
      </section>

      <div class="grid gap-6 xl:grid-cols-[320px_minmax(0,1fr)]">
        <aside class="space-y-6">
          <section class="glass-panel rounded-[2rem] p-5">
            <div class="flex items-start justify-between gap-4">
              <div>
                <p class="section-kicker !text-brand-700">Access Rail</p>
                <h2 class="mt-3 font-display text-2xl font-bold tracking-tight text-ink-900">
                  Tenant context
                </h2>
              </div>

              <span class="surface-chip">{$authSession?.user.role ?? '-'}</span>
            </div>

            <div class="mt-5 rounded-[1.6rem] bg-canvas-50 px-4 py-4">
              <p class="text-sm font-semibold text-ink-900">Notification scope</p>
              <p class="mt-2 text-xs leading-5 text-ink-500">{notificationScope.description}</p>
              <div class="mt-4 flex flex-wrap items-center gap-2">
                <span class="surface-chip">{notificationScope.label}</span>
                <span class="surface-chip">
                  {notificationLoading ? 'syncing' : `${formatNumber(unreadNotificationCount)} unread`}
                </span>
              </div>
            </div>

            <div class="mt-4">
              <ThemeToggle />
            </div>

            <div class="mt-4">
              <Button variant="outline" size="lg" class="w-full" onclick={signOut}>
                Logout
              </Button>
            </div>
          </section>

          <section class="glass-panel rounded-[2rem] p-5">
            <div class="flex items-start justify-between gap-4">
              <div>
                <p class="section-kicker !text-brand-700">Quick Switch</p>
                <h2 class="mt-3 font-display text-2xl font-bold tracking-tight text-ink-900">
                  Active store
                </h2>
              </div>

              <span class="surface-chip">{formatNumber(storeDirectorySummary.total_count)} store</span>
            </div>

            {#if storeDirectoryLoading}
              <div class="mt-5 animate-pulse rounded-[1.6rem] bg-canvas-50 px-4 py-5">
                <div class="h-3 w-24 rounded-full bg-white/80"></div>
                <div class="mt-3 h-11 rounded-2xl bg-white/80"></div>
              </div>
            {:else if storeDirectoryError !== ''}
              <div class="mt-5">
                <Notice tone="warning" message={storeDirectoryError} />
              </div>
            {:else if accessibleStores.length === 0}
              <div class="mt-5 rounded-[1.6rem] bg-canvas-50 px-4 py-5 text-sm leading-6 text-ink-700">
                Belum ada toko di scope sesi ini.
              </div>
            {:else}
              <div class="mt-5 space-y-4">
                <label class="block space-y-2">
                  <span class="text-sm font-medium text-ink-700">Cari store untuk switch context</span>
                  <input
                    bind:value={storeQuery}
                    type="search"
                    placeholder="Cari nama, slug, atau callback URL..."
                    class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                  />
                </label>

                <div class="flex flex-wrap gap-2">
                  <Button variant="brand" size="sm" onclick={applyStoreDirectorySearch}>
                    Search
                  </Button>
                  <Button variant="outline" size="sm" onclick={resetStoreDirectorySearch}>
                    Reset
                  </Button>
                  <span class="surface-chip">{formatNumber(accessibleStores.length)} on page</span>
                  <span class="surface-chip">{formatNumber(storeDirectorySummary.low_balance_count)} low balance</span>
                </div>

                <label class="block space-y-2">
                  <span class="text-sm font-medium text-ink-700">Store aktif untuk command flow</span>
                  <select
                    bind:value={selectedStoreID}
                    class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                    onchange={(event) => switchStore((event.currentTarget as HTMLSelectElement).value)}
                  >
                    {#each accessibleStores as store}
                      <option value={store.id}>{store.name} · {store.slug}</option>
                    {/each}
                  </select>
                </label>

                <PaginationControls
                  bind:page={storePage}
                  bind:pageSize={storePageSize}
                  totalItems={storeDirectorySummary.total_count}
                  pageSizeOptions={[6, 12, 24]}
                />
              </div>
            {/if}

            {#if currentStore}
              <div class="mt-4 rounded-[1.6rem] border border-ink-100 bg-white/74 px-4 py-4">
                <p class="text-sm font-semibold text-ink-900">{currentStore.name}</p>
                <div class="mt-3 space-y-2 text-xs leading-5 text-ink-500">
                  <p>Balance: {formatCurrency(currentStore.current_balance)}</p>
                  <p>
                    Threshold:
                    {currentStore.low_balance_threshold
                      ? formatCurrency(currentStore.low_balance_threshold)
                      : '-'}
                  </p>
                  <p>Staff: {formatNumber(currentStore.staff_count)}</p>
                </div>
              </div>
            {/if}
          </section>

          {#if storeDirectorySummary.low_balance_count > 0}
            <section class="glass-panel rounded-[2rem] p-5">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <p class="section-kicker !text-accent-700">Alert</p>
                  <h2 class="mt-3 font-display text-2xl font-bold tracking-tight text-ink-900">
                    Low balance
                  </h2>
                </div>

                <span class="surface-chip">{formatNumber(storeDirectorySummary.low_balance_count)} store</span>
              </div>

              <p class="mt-3 text-sm leading-6 text-ink-700">
                Store di bawah threshold akan ikut memicu notification event dan bisa berdampak ke
                deposit atau withdrawal flow.
              </p>

              <div class="mt-4 space-y-3">
                {#each visibleLowBalanceStores.slice(0, 3) as store}
                  <div class="rounded-[1.4rem] border border-amber-200/60 bg-linear-to-r from-accent-100/45 to-white px-4 py-4">
                    <p class="text-sm font-semibold text-ink-900">{store.name}</p>
                    <p class="mt-1 text-xs text-ink-500">{formatCurrency(store.current_balance)}</p>
                  </div>
                {/each}
              </div>

              {#if visibleLowBalanceStores.length === 0}
                <div class="mt-4 rounded-[1.4rem] border border-ink-100 bg-white/76 px-4 py-4 text-xs leading-6 text-ink-600">
                  Store low balance ada di scope backend, tetapi tidak sedang tampil pada page switcher saat ini. Gunakan search atau pagination untuk menemukannya.
                </div>
              {/if}

              {#if currentStore && selectedStoreIsLowBalance}
                <div class="mt-4">
                  <Notice
                    tone="warning"
                    message={`Store aktif saat ini juga low balance: ${currentStore.name}.`}
                  />
                </div>
              {/if}
            </section>
          {/if}
        </aside>

        <main class="min-w-0 space-y-6" id="app-main" tabindex="-1">
          <slot />
        </main>
      </div>
    </div>
  </div>
{:else}
  <div class="matrix-page" data-app-shell="loading">
    <div class="shell-width mx-auto min-h-screen py-8">
      <div class="surface-dark surface-grid rounded-[2.4rem] px-6 py-8 text-white">
        <p class="section-kicker">Session Handshake</p>
        <h1 class="mt-3 font-display text-3xl font-bold tracking-tight">Memeriksa sesi dashboard...</h1>
        <p class="mt-3 max-w-xl text-sm leading-7 text-white/68">
          Shell sedang memuat profile user, scope store, notification counter, dan koneksi realtime.
        </p>
      </div>
    </div>
  </div>
{/if}
