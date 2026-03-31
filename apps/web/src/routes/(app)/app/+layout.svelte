<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import Button from '$lib/components/ui/button/button.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import {
    authSession,
    initializeAuthSession,
    logoutCurrentSession,
    syncProfile
  } from '$lib/auth/client';
  import {
    connectRealtime,
    disconnectRealtime,
    realtimeState
  } from '$lib/realtime/client';
  import {
    fetchUnreadNotificationCount,
    isNotificationEvent,
    resolveNotificationScope,
    subscribeNotificationsChanged
  } from '$lib/notifications/client';
  import {
    fetchStores,
    isStoreLowBalance,
    parseMoney,
    type Store
  } from '$lib/stores/client';
  import {
    hydratePreferredStoreID,
    pickPreferredStoreID,
    preferredStoreID,
    setPreferredStoreID
  } from '$lib/stores/preferences';

  let ready = false;
  let storeDirectoryLoading = true;
  let storeDirectoryError = '';
  let accessibleStores: Store[] = [];
  let selectedStoreID = '';
  let unreadNotificationCount = 0;
  let notificationLoading = false;
  let lastNotificationEventKey: string | null = null;
  let lastNotificationScopeKey = '';

  $: role = $authSession?.user.role ?? '';
  $: currentStore = accessibleStores.find((store) => store.id === selectedStoreID) ?? null;
  $: lowBalanceStores = accessibleStores.filter((store) => isStoreLowBalance(store));
  $: selectedStoreIsLowBalance = currentStore ? isStoreLowBalance(currentStore) : false;
  $: notificationScope = resolveNotificationScope(role, selectedStoreID);
  $: notificationBadge =
    unreadNotificationCount > 99 ? '99+' : unreadNotificationCount > 0 ? String(unreadNotificationCount) : '';
  $: nav = [
    { href: '/app', label: 'Dashboard' },
    { href: '/app/notifications', label: 'Notifications', badge: notificationBadge },
    { href: '/app/stores', label: 'Stores' },
    { href: '/app/catalog', label: 'Catalog' },
    { href: '/app/members', label: 'Members' },
    ...(role === 'karyawan'
        ? []
        : [
          { href: '/app/topups', label: 'Topups' },
          { href: '/app/bank-accounts', label: 'Bank Accounts' },
          { href: '/app/withdrawals', label: 'Withdrawals' },
          { href: '/app/audit', label: 'Audit' }
        ]),
    { href: '/app/security', label: 'Security' },
    { href: '/app/chat', label: 'Global Chat' },
    { href: '/', label: 'Back to Public' }
  ];

  onMount(() => {
    let active = true;
    hydratePreferredStoreID();

    const unsubscribeStorePreference = preferredStoreID.subscribe((storeID) => {
      if (!active) {
        return;
      }

      if (storeID !== '' && accessibleStores.some((store) => store.id === storeID)) {
        selectedStoreID = storeID;
      }
    });

    async function loadAccessibleStores() {
      storeDirectoryLoading = true;
      storeDirectoryError = '';

      const response = await fetchStores();
      if (!active) {
        return;
      }

      if (!response.status || response.message !== 'SUCCESS') {
        storeDirectoryError =
          response.message === 'FORBIDDEN'
            ? 'Store switch tidak tersedia untuk role ini.'
            : 'Store switch belum bisa dimuat. Halaman lain tetap bisa dipakai.';
        accessibleStores = [];
        selectedStoreID = '';
        storeDirectoryLoading = false;
        return;
      }

      accessibleStores = response.data ?? [];
      selectedStoreID = pickPreferredStoreID(accessibleStores, selectedStoreID);
      setPreferredStoreID(selectedStoreID);
        storeDirectoryLoading = false;
    }

    async function loadUnreadNotifications() {
      if (!active) {
        return;
      }

      if (!notificationScope.ready) {
        unreadNotificationCount = 0;
        notificationLoading = false;
        return;
      }

      notificationLoading = true;
      const response = await fetchUnreadNotificationCount(notificationScope.params);
      if (!active) {
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
      if (!active) {
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
      if (!active || !notificationScope.ready) {
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
      active = false;
      unsubscribeStorePreference();
      unsubscribeRealtime();
      unsubscribeNotificationsChanged();
      disconnectRealtime();
    };
  });

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
  }

  function formatCurrency(value: string | null | undefined) {
    return new Intl.NumberFormat('id-ID', {
      style: 'currency',
      currency: 'IDR',
      minimumFractionDigits: 0,
      maximumFractionDigits: 0
    }).format(parseMoney(value));
  }

  function formatThreshold(value: string | null | undefined) {
    if ((value ?? '').trim() === '') {
      return '-';
    }

    return formatCurrency(value);
  }
</script>

{#if ready}
  <div class="shell-width mx-auto flex min-h-screen flex-col gap-6 py-6 lg:flex-row">
    <aside class="glass-panel w-full rounded-4xl p-5 lg:sticky lg:top-6 lg:h-[calc(100vh-3rem)] lg:w-80">
      <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">App Shell</p>
      <h1 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">onixggr</h1>
      <p class="mt-3 text-sm leading-6 text-ink-700">
        Area app sekarang menutup auth, store management, topup, withdraw, store members, audit
        viewer, dan security hardening dari blueprint awal.
      </p>

      {#if $authSession}
        <div class="mt-6 rounded-3xl bg-canvas-100 px-4 py-4 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Signed In</p>
          <p class="mt-1">{$authSession.user.username}</p>
          <p>{$authSession.user.role}</p>
        </div>

        <div class="mt-4 rounded-3xl border border-ink-100 px-4 py-4 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Realtime</p>
          <p class="mt-1 uppercase tracking-[0.18em] text-brand-700">{$realtimeState.status}</p>
          <p class="mt-2 text-xs text-ink-500">
            {$realtimeState.channels.length} channel aktif
          </p>
        </div>

        <div class="mt-4 rounded-3xl border border-ink-100 px-4 py-4 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Quick Store Switch</p>
          <p class="mt-1 text-xs leading-5 text-ink-500">
            Dipakai sebagai default di halaman members, topups, bank accounts, dan withdrawals.
          </p>

          {#if storeDirectoryLoading}
            <div class="mt-4 animate-pulse rounded-2xl bg-canvas-100 px-4 py-4">
              <div class="h-3 w-24 rounded-full bg-white/80"></div>
              <div class="mt-3 h-10 rounded-2xl bg-white/80"></div>
            </div>
          {:else if storeDirectoryError !== ''}
            <div class="mt-4">
              <Notice tone="warning" message={storeDirectoryError} />
            </div>
          {:else if accessibleStores.length === 0}
            <div class="mt-4 rounded-2xl bg-canvas-100 px-4 py-4 text-xs leading-5 text-ink-600">
              Belum ada toko di scope sesi ini.
            </div>
          {:else}
            <label class="mt-4 block space-y-2">
              <span class="text-xs font-semibold uppercase tracking-[0.18em] text-ink-500">
                Store aktif
              </span>
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

            {#if currentStore}
              <div class="mt-4 rounded-2xl bg-canvas-100 px-4 py-4 text-xs leading-5 text-ink-700">
                <p class="font-semibold text-ink-900">{currentStore.name}</p>
                <p>Balance: {formatCurrency(currentStore.current_balance)}</p>
                <p>Threshold: {formatThreshold(currentStore.low_balance_threshold)}</p>
              </div>
            {/if}
          {/if}
        </div>

        <div class="mt-4 rounded-3xl border border-ink-100 px-4 py-4 text-sm text-ink-700">
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="font-semibold text-ink-900">Notifications</p>
              <p class="mt-1 text-xs leading-5 text-ink-500">{notificationScope.label}</p>
            </div>
            {#if notificationLoading}
              <span class="rounded-full bg-canvas-100 px-3 py-1 text-xs font-semibold text-ink-500">
                ...
              </span>
            {:else if notificationBadge !== ''}
              <span class="rounded-full bg-accent-100 px-3 py-1 text-xs font-semibold text-accent-800">
                {notificationBadge}
              </span>
            {/if}
          </div>
          <p class="mt-3 text-xs leading-5 text-ink-500">{notificationScope.description}</p>
          <a
            class="mt-3 inline-flex text-xs font-semibold uppercase tracking-[0.18em] text-brand-700 underline-offset-4 hover:underline"
            href="/app/notifications"
          >
            Open feed
          </a>
        </div>

        {#if lowBalanceStores.length > 0}
          <div class="mt-4 rounded-3xl border border-amber-200 bg-amber-50 px-4 py-4 text-sm text-amber-900">
            <p class="font-semibold">Low Balance Alert</p>
            <p class="mt-2 leading-6">
              {lowBalanceStores.length} toko berada di threshold atau di bawah threshold saldo.
            </p>
            {#if currentStore && selectedStoreIsLowBalance}
              <p class="mt-2 text-xs leading-5">
                Store aktif saat ini juga low balance: {currentStore.name} dengan saldo
                {formatCurrency(currentStore.current_balance)}.
              </p>
            {/if}
            <a class="mt-3 inline-flex text-xs font-semibold uppercase tracking-[0.18em] text-amber-800 underline-offset-4 hover:underline" href="/app/stores">
              Review threshold
            </a>
          </div>
        {/if}
      {/if}

      <nav class="mt-8 space-y-2">
        {#each nav as item}
          <a
            class="flex items-center justify-between gap-3 rounded-2xl border border-transparent px-4 py-3 text-sm font-medium text-ink-700 transition hover:border-ink-100 hover:bg-canvas-100 hover:text-ink-900"
            href={item.href}
          >
            <span>{item.label}</span>
            {#if item.badge}
              <span class="rounded-full bg-accent-100 px-3 py-1 text-xs font-semibold text-accent-800">
                {item.badge}
              </span>
            {/if}
          </a>
        {/each}
      </nav>

      <div class="mt-8">
        <Button variant="outline" size="lg" class="w-full" onclick={signOut}>
          Logout
        </Button>
      </div>
    </aside>

    <main class="min-w-0 flex-1">
      <slot />
    </main>
  </div>
{:else}
  <div class="shell-width mx-auto min-h-screen py-10">
    <div class="glass-panel rounded-4xl px-6 py-8">
      <p class="text-sm text-ink-700">Memeriksa session dashboard...</p>
    </div>
  </div>
{/if}
