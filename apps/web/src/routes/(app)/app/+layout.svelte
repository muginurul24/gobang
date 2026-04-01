<script lang="ts">
  import { browser } from '$app/environment';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { onMount } from 'svelte';

  import Button from '$lib/components/ui/button/button.svelte';
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
  let sessionBootstrapWarning = '';
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
  let storePageSize = 18;
  let selectedStoreID = '';
  let selectedStoreSummary: Store | null = null;
  let unreadNotificationCount = 0;
  let notificationLoading = false;
  let lastNotificationEventKey: string | null = null;
  let lastNotificationScopeKey = '';
  let lastStoreDirectoryKey = '';
  let viewActive = false;
  let navOpen = false;
  let navPinned = false;
  let isWideViewport = false;
  let sidebarVisible = false;

  const navPreferenceKey = 'onixggr.shell.nav-pinned';

  $: role = $authSession?.user.role ?? '';
  $: currentStore =
    accessibleStores.find((store) => store.id === selectedStoreID) ??
    (selectedStoreSummary?.id === selectedStoreID ? selectedStoreSummary : null);
  $: visibleLowBalanceStores = accessibleStores.filter((store) => isStoreLowBalance(store));
  $: selectedStoreIsLowBalance = currentStore ? isStoreLowBalance(currentStore) : false;
  $: storePageCount = Math.max(1, Math.ceil((storeDirectorySummary.total_count || 0) / storePageSize));
  $: canGoPrevStorePage = storePage > 1;
  $: canGoNextStorePage = storePage < storePageCount;
  $: notificationScope = resolveNotificationScope(role, selectedStoreID);
  $: notificationBadge = unreadNotificationCount > 99
    ? '99+'
    : unreadNotificationCount > 0
      ? String(unreadNotificationCount)
      : '';
  $: currentPath = normalizePath($page.url.pathname);
  $: nav = [
    { href: '/app', label: 'Dashboard', description: 'realtime cards' },
    ...(
      role === 'karyawan'
        ? []
        : [{ href: '/app/onboarding', label: 'Onboarding', description: 'dev -> owner -> store' }]
    ),
    { href: '/app/notifications', label: 'Notifications', description: 'event stream', badge: notificationBadge },
    ...(
      role === 'dev' || role === 'superadmin'
        ? [
            { href: '/app/users', label: 'Users', description: 'owner onboarding' },
            { href: '/app/ops', label: 'Ops', description: 'health + callbacks' }
          ]
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
  ].map((item) => ({
    ...item,
    active: isActivePath(currentPath, item.href)
  }));
  $: currentNavItem = nav.find((item) => item.active) ?? nav[0];
  $: currentPageTitle = currentNavItem?.label ?? 'Dashboard';
  $: currentPageDescription = currentNavItem?.description ?? 'realtime cards';
  $: sidebarVisible = isWideViewport ? navPinned : navOpen;

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
    const mediaQuery = window.matchMedia('(min-width: 1280px)');

    function syncViewportState(matches: boolean) {
      isWideViewport = matches;
      if (matches) {
        const storedPreference = browser ? window.localStorage.getItem(navPreferenceKey) : null;
        navPinned = storedPreference === null ? true : storedPreference === 'true';
        navOpen = false;
        return;
      }

      navOpen = false;
    }

    syncViewportState(mediaQuery.matches);
    const handleViewportChange = (event: MediaQueryListEvent) => {
      syncViewportState(event.matches);
    };
    mediaQuery.addEventListener('change', handleViewportChange);

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
        if (profile.message === 'UNAUTHORIZED') {
          disconnectRealtime();
          await goto('/login');
          return;
        }

        sessionBootstrapWarning =
          'Profile sesi belum tersinkron penuh. Dashboard tetap memakai sesi lokal dan akan mencoba sinkron lagi saat request berikutnya.';
      } else {
        sessionBootstrapWarning = '';
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
      mediaQuery.removeEventListener('change', handleViewportChange);
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

  function toggleSidebar() {
    if (isWideViewport) {
      navPinned = !navPinned;
      if (browser) {
        window.localStorage.setItem(navPreferenceKey, navPinned ? 'true' : 'false');
      }
      return;
    }

    navOpen = !navOpen;
  }

  function closeSidebar() {
    if (!isWideViewport) {
      navOpen = false;
    }
  }

  function switchStore(storeID: string) {
    selectedStoreID = storeID;
    setPreferredStoreID(storeID);
    void syncSelectedStoreSummary(storeID);
    closeSidebar();
  }

  async function goToPreviousStorePage() {
    if (!canGoPrevStorePage) {
      return;
    }

    storePage -= 1;
    lastStoreDirectoryKey = '';
    await loadAccessibleStores();
  }

  async function goToNextStorePage() {
    if (!canGoNextStorePage) {
      return;
    }

    storePage += 1;
    lastStoreDirectoryKey = '';
    await loadAccessibleStores();
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

  function normalizePath(pathname: string) {
    if (pathname.length > 1 && pathname.endsWith('/')) {
      return pathname.slice(0, -1);
    }

    return pathname;
  }

  function isActivePath(pathname: string, href: string) {
    if (href === '/') {
      return pathname === '/';
    }

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
    {#if !isWideViewport && sidebarVisible}
      <button
        aria-label="Close navigation"
        class="shell-sidebar-scrim"
        onclick={closeSidebar}
      ></button>
    {/if}

    <div class="shell-width shell-frame mx-auto min-h-screen gap-6 pb-10 pt-4 sm:pt-6" data-sidebar={sidebarVisible}>
      <aside class="shell-sidebar" data-open={sidebarVisible}>
        <div class="shell-sidebar__panel soft-scroll">
          <section class="glass-panel shell-sidebar-shell rounded-[2rem] p-5">
            <div class="flex items-start justify-between gap-4">
              <div>
                <p class="section-kicker !text-brand-700">Navigation</p>
                <h2 class="mt-3 font-display text-[1.85rem] font-bold tracking-tight text-ink-900">
                  Control lanes
                </h2>
                <p class="mt-2 text-sm leading-6 text-ink-700">
                  Jalur utama untuk berpindah lane. Context store, notification, dan action rail
                  sudah dipindah ke area utama agar sidebar tetap ringkas di desktop.
                </p>
              </div>

              <div class="flex flex-wrap justify-end gap-2">
                <span class="surface-chip">{$authSession?.user.role ?? '-'}</span>
                {#if !isWideViewport}
                  <Button variant="outline" size="sm" onclick={closeSidebar}>Close</Button>
                {/if}
              </div>
            </div>

            <div class="shell-sidebar-user mt-5">
              <div class="space-y-2">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="surface-chip">{$authSession?.user.username ?? '-'}</span>
                  <span class="surface-chip">{role || 'guest'}</span>
                </div>
                <p class="text-sm leading-6 text-ink-700">
                  {$authSession?.user.email ?? 'unknown'}
                </p>
              </div>

              <ThemeToggle compact={true} />
            </div>

            <nav class="nav-cluster mt-5">
              {#each nav as item}
                <a
                  aria-current={item.active ? 'page' : undefined}
                  class="app-nav-link"
                  data-active={item.active}
                  href={item.href}
                  onclick={closeSidebar}
                >
                  <span class="app-nav-link__marker" aria-hidden="true"></span>
                  <span class="app-nav-link__content">
                    <span class="app-nav-link__label">{item.label}</span>
                    <span class="app-nav-link__meta">{item.description}</span>
                  </span>
                  {#if item.active}
                    <span class="app-nav-link__state">Active</span>
                  {/if}
                  {#if item.badge}
                    <span class="app-nav-link__badge">{item.badge}</span>
                  {/if}
                </a>
              {/each}
            </nav>

            <div class="mt-5 grid gap-3">
              <a class="surface-chip w-fit" href="/" onclick={closeSidebar}>Open public site</a>
              <Button variant="outline" size="lg" class="w-full" onclick={signOut}>
                Logout
              </Button>
            </div>
          </section>
        </div>
      </aside>

      <div class="shell-main min-w-0 space-y-6">
        <section class="shell-utility-bar">
          <div class="shell-utility-bar__group">
            <Button variant="outline" size="sm" onclick={toggleSidebar}>
              {isWideViewport ? (sidebarVisible ? 'Hide Nav' : 'Show Nav') : (sidebarVisible ? 'Close Menu' : 'Open Menu')}
            </Button>
            <span class="surface-chip">lane {currentPageTitle}</span>
            <span class="surface-chip">realtime {realtimeLabel()}</span>
            <span class="surface-chip">store {currentStore?.slug ?? 'platform-wide'}</span>
          </div>

          <div class="shell-utility-bar__group">
            {#if notificationBadge !== ''}
              <span class="surface-chip">{notificationBadge} unread</span>
            {/if}
            <span class="surface-chip">role {role || 'guest'}</span>
          </div>
        </section>

        <section class="shell-header glass-panel rounded-[2.15rem] p-5 sm:p-6">
          <div class="shell-header__main">
            <div class="space-y-3">
              <div class="flex flex-wrap items-center gap-2">
                <span class="section-kicker !text-brand-700">Current Lane</span>
                <span class="surface-chip">{currentPath}</span>
                <span class="surface-chip">{$authSession?.user.username ?? '-'}</span>
              </div>

              <div class="space-y-2">
                <div class="flex flex-wrap items-center gap-3">
                  <h1 class="font-display text-3xl font-bold tracking-tight text-ink-900 sm:text-[2.5rem]">
                    {currentPageTitle}
                  </h1>
                  <span class="surface-chip">role {role || 'guest'}</span>
                  <span class="surface-chip">realtime {realtimeLabel()}</span>
                </div>
                <p class="max-w-3xl text-sm leading-7 text-ink-700 sm:text-base">
                  {currentPageDescription}. {pageSummary()}
                </p>
              </div>
            </div>

            <div class="shell-header__rail">
              <article class="shell-header__metric">
                <span class="shell-header__metric-label">Notifications</span>
                <strong class="shell-header__metric-value">
                  {notificationBadge === '' ? '0' : notificationBadge}
                </strong>
                <span class="shell-header__metric-meta">{notificationScope.label}</span>
              </article>

              <article class="shell-header__metric">
                <span class="shell-header__metric-label">Store focus</span>
                <strong class="shell-header__metric-value">{currentStore?.name ?? 'Platform-wide'}</strong>
                <span class="shell-header__metric-meta">
                  {currentStore ? formatCurrency(currentStore.current_balance) : 'lintas store'}
                </span>
              </article>

              <article class="shell-header__metric">
                <span class="shell-header__metric-label">Transport</span>
                <strong class="shell-header__metric-value">{realtimeLabel()}</strong>
                <span class="shell-header__metric-meta">
                  {formatNumber($realtimeState.channels.length)} channel aktif
                </span>
              </article>
            </div>
          </div>
        </section>

        <section class="shell-context-grid">
          <article class="glass-panel shell-context-card rounded-[2rem] p-5">
            <div class="flex items-end justify-between gap-3">
              <div>
                <p class="section-kicker !text-brand-700">Store context</p>
                <h2 class="mt-2 font-display text-xl font-bold tracking-tight text-ink-900">
                  Active store rail
                </h2>
              </div>
              <span class="surface-chip">{formatNumber(storeDirectorySummary.total_count)} scoped</span>
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
                Belum ada toko di scope sesi ini. Gunakan onboarding untuk provision owner dan
                tenant pertama, lalu selector ini akan aktif otomatis.
                <div class="mt-4 flex flex-wrap gap-2">
                  <a class="surface-chip" href={role === 'karyawan' ? '/app/stores' : '/app/onboarding'}>
                    {role === 'karyawan' ? 'Open stores' : 'Open onboarding'}
                  </a>
                  <a class="surface-chip" href="/app/api-docs">Open API docs</a>
                </div>
              </div>
            {:else}
              <div class="mt-5 grid gap-4 xl:grid-cols-[minmax(0,1fr)_15rem] xl:items-end">
                <label class="field-stack">
                  <span class="field-label">Cari store untuk command flow</span>
                  <input
                    bind:value={storeQuery}
                    type="search"
                    placeholder="Cari nama, slug, atau callback URL..."
                    class="field-input"
                  />
                </label>

                <label class="field-stack">
                  <span class="field-label">Store aktif</span>
                  <select
                    bind:value={selectedStoreID}
                    class="field-select"
                    onchange={(event) => switchStore((event.currentTarget as HTMLSelectElement).value)}
                  >
                    {#each accessibleStores as store}
                      <option value={store.id}>{store.name} · {store.slug}</option>
                    {/each}
                  </select>
                </label>
              </div>
              <div class="shell-context-card__toolbar mt-4">
                <div class="toolbar-actions">
                  <Button variant="brand" size="sm" onclick={applyStoreDirectorySearch}>
                    Search
                  </Button>
                  <Button variant="outline" size="sm" onclick={resetStoreDirectorySearch}>
                    Reset
                  </Button>
                  <a class="surface-chip" href="/app/stores">Open stores</a>
                </div>

                <div class="shell-context-card__pager">
                  <span class="surface-chip">page {storePage}/{storePageCount}</span>
                  <Button variant="outline" size="sm" onclick={goToPreviousStorePage} disabled={!canGoPrevStorePage}>
                    Prev
                  </Button>
                  <Button variant="outline" size="sm" onclick={goToNextStorePage} disabled={!canGoNextStorePage}>
                    Next
                  </Button>
                </div>
              </div>

              <div class="mt-4 flex flex-wrap gap-2">
                <span class="surface-chip">{formatNumber(accessibleStores.length)} loaded</span>
                <span class="surface-chip">{formatNumber(storeDirectorySummary.low_balance_count)} low balance</span>
                {#if currentStore}
                  <span class="surface-chip">{currentStore.slug}</span>
                  <span class="surface-chip">{formatCurrency(currentStore.current_balance)}</span>
                {/if}
              </div>
            {/if}
          </article>

          <article class="glass-panel shell-context-card rounded-[2rem] p-5">
            <div class="flex items-end justify-between gap-3">
              <div>
                <p class="section-kicker !text-brand-700">Realtime scope</p>
                <h2 class="mt-2 font-display text-xl font-bold tracking-tight text-ink-900">
                  Notification and transport
                </h2>
              </div>
              <span class="surface-chip">{notificationScope.label}</span>
            </div>

            <div class="mt-4 grid gap-3 sm:grid-cols-2">
              <div class="rounded-[1.5rem] bg-canvas-50 px-4 py-4">
                <p class="text-xs font-semibold uppercase tracking-[0.24em] text-ink-500">Unread</p>
                <p class="mt-2 font-display text-2xl font-semibold tracking-tight text-ink-900">
                  {notificationBadge === '' ? '0' : notificationBadge}
                </p>
                <p class="mt-2 text-xs leading-5 text-ink-500">
                  {notificationLoading ? 'Counter sedang disinkronkan.' : notificationScope.description}
                </p>
              </div>

              <div class="rounded-[1.5rem] bg-canvas-50 px-4 py-4">
                <p class="text-xs font-semibold uppercase tracking-[0.24em] text-ink-500">Transport</p>
                <p class="mt-2 font-display text-2xl font-semibold tracking-tight text-ink-900">
                  {realtimeLabel()}
                </p>
                <p class="mt-2 text-xs leading-5 text-ink-500">
                  {formatNumber($realtimeState.channels.length)} channel aktif untuk sesi ini.
                </p>
              </div>
            </div>

            {#if currentStore}
              <div class="mt-4 flex flex-wrap gap-2">
                <span class="surface-chip">{currentStore.name}</span>
                <span class="surface-chip">staff {formatNumber(currentStore.staff_count)}</span>
                <span class="surface-chip">
                  threshold {currentStore.low_balance_threshold
                    ? formatCurrency(currentStore.low_balance_threshold)
                    : '-'}
                </span>
              </div>
            {/if}
          </article>

          <article class="glass-panel shell-context-card rounded-[2rem] p-5">
            <div class="flex items-end justify-between gap-3">
              <div>
                <p class="section-kicker !text-brand-700">Action lane</p>
                <h2 class="mt-2 font-display text-xl font-bold tracking-tight text-ink-900">
                  Next best action
                </h2>
              </div>
              <span class="surface-chip">role {role || 'guest'}</span>
            </div>

            <p class="mt-4 text-sm leading-6 text-ink-700">
              {#if role === 'dev' || role === 'superadmin'}
                Provision owner lalu pindah ke Ops saat callback atau health bermasalah.
              {:else if role === 'owner'}
                Buat store, atur callback, siapkan staff, lalu lanjutkan integrasi.
              {:else}
                Fokus ke store yang sudah diassign dan pantau notification stream.
              {/if}
            </p>

            <div class="mt-4 flex flex-wrap gap-3">
              {#if role === 'dev' || role === 'superadmin'}
                <a class="surface-chip" href="/app/onboarding">Open onboarding</a>
                <a class="surface-chip" href="/app/users">Provision owner</a>
                <a class="surface-chip" href="/app/ops">Open ops</a>
              {:else if role === 'owner'}
                <a class="surface-chip" href="/app/onboarding">Create first store</a>
                <a class="surface-chip" href="/app/stores">Manage stores</a>
                <a class="surface-chip" href="/app/api-docs">Read API docs</a>
              {:else}
                <a class="surface-chip" href="/app/stores">View stores</a>
                <a class="surface-chip" href="/app/notifications">Open notifications</a>
                <a class="surface-chip" href="/app/security">Open security</a>
              {/if}
            </div>

            {#if storeDirectorySummary.low_balance_count > 0}
              <div class="mt-4 rounded-[1.5rem] border border-amber-200/60 bg-linear-to-r from-accent-100/40 to-white px-4 py-4 text-sm leading-6 text-ink-700">
                Ada {formatNumber(storeDirectorySummary.low_balance_count)} store low balance di scope
                sesi ini. Gunakan Stores atau dashboard cards untuk triage lebih lanjut.
              </div>
            {/if}
          </article>
        </section>

        <main class="min-w-0 space-y-6" id="app-main" tabindex="-1">
          {#if sessionBootstrapWarning !== ''}
            <Notice
              tone="warning"
              title="Session Sync"
              message={sessionBootstrapWarning}
            />
          {/if}
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
