<script lang="ts">
  import { onMount } from 'svelte';

  import { authSession } from '$lib/auth/client';
  import {
    fetchDashboardCards,
    type DashboardPlatformMetrics,
    type DashboardStoreMetrics
  } from '$lib/dashboard/client';
  import { realtimeState } from '$lib/realtime/client';

  const relevantRealtimeEvents = new Set([
    'member_payment.success',
    'store_topup.success',
    'withdraw.success',
    'withdraw.failed',
    'callback.delivery_failed',
    'game.deposit.success',
    'game.withdraw.success',
    'store.low_balance'
  ]);

  let loading = true;
  let errorMessage: string | null = null;
  let storeMetrics: DashboardStoreMetrics | null = null;
  let platformMetrics: DashboardPlatformMetrics | null = null;
  let lastSyncedAt: string | null = null;
  let requestInFlight = false;
  let lastRealtimeKey: string | null = null;
  let lastConnectionID: string | null = null;

  onMount(() => {
    let active = true;

    async function loadCards() {
      if (!active || requestInFlight) {
        return;
      }

      requestInFlight = true;
      errorMessage = null;

      const response = await fetchDashboardCards();
      requestInFlight = false;
      if (!active) {
        return;
      }

      if (!response.status || response.message !== 'SUCCESS') {
        errorMessage = response.message;
        loading = false;
        return;
      }

      storeMetrics = response.data.store_metrics ?? null;
      platformMetrics = response.data.platform_metrics ?? null;
      lastSyncedAt = new Date().toLocaleTimeString('id-ID', {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
      });
      loading = false;
    }

    const interval = window.setInterval(() => {
      void loadCards();
    }, 30000);

    const unsubscribe = realtimeState.subscribe((snapshot) => {
      if (!active) {
        return;
      }

      if (snapshot.connection_id && snapshot.connection_id !== lastConnectionID) {
        lastConnectionID = snapshot.connection_id;
        void loadCards();
      }

      const latestEvent = snapshot.events[0];
      if (!latestEvent) {
        return;
      }

      const eventKey = `${latestEvent.created_at}:${latestEvent.channel}:${latestEvent.type}`;
      if (eventKey === lastRealtimeKey) {
        return;
      }
      lastRealtimeKey = eventKey;

      if (relevantRealtimeEvents.has(latestEvent.type)) {
        void loadCards();
      }
    });

    void loadCards();

    return () => {
      active = false;
      window.clearInterval(interval);
      unsubscribe();
    };
  });

  function formatCurrency(value: string | null | undefined) {
    const amount = Number(value ?? '0');
    return new Intl.NumberFormat('id-ID', {
      style: 'currency',
      currency: 'IDR',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2
    }).format(Number.isFinite(amount) ? amount : 0);
  }

  function formatPercent(value: number | null | undefined) {
    return `${(value ?? 0).toFixed(2)}%`;
  }

  function liveMode(status: string) {
    return status === 'connected' ? 'Realtime aktif' : 'Fallback polling 30s';
  }
</script>

<svelte:head>
  <title>App | onixggr</title>
</svelte:head>

<section class="space-y-6">
  <div class="glass-panel rounded-4xl px-6 py-7">
    <p class="text-xs font-semibold uppercase tracking-[0.24em] text-accent-700">
      Dashboard Cards
    </p>
    <h2 class="mt-3 font-display text-4xl font-bold tracking-tight text-ink-900">
      Angka operasional yang mengikuti scope role dashboard
    </h2>
    <p class="mt-3 max-w-3xl text-sm leading-7 text-ink-700">
      Ringkasan ini mengambil agregat backend sesuai blueprint: owner atau karyawan hanya melihat
      toko yang memang bisa mereka akses, sedangkan dev atau superadmin melihat kartu platform.
    </p>

    {#if $authSession}
      <div class="mt-6 grid gap-4 lg:grid-cols-[1fr_18rem]">
        <div class="rounded-3xl bg-canvas-100 px-5 py-4 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Current Session</p>
          <p class="mt-1">{$authSession.user.email}</p>
          <p>Role: {$authSession.user.role}</p>
        </div>
        <div class="rounded-3xl border border-ink-100 px-5 py-4 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Sync Mode</p>
          <p class="mt-1 uppercase tracking-[0.18em] text-brand-700">
            {liveMode($realtimeState.status)}
          </p>
          <p class="mt-2 text-xs text-ink-500">Last sync: {lastSyncedAt ?? 'belum ada'}</p>
        </div>
      </div>
    {/if}
  </div>

  {#if errorMessage}
    <article class="rounded-3xl border border-danger/30 bg-danger/10 px-5 py-4 text-sm text-danger">
      Gagal memuat dashboard cards: {errorMessage}
    </article>
  {/if}

  {#if loading}
    <div class="grid gap-4 md:grid-cols-3">
      {#each Array(6) as _, index}
        <article class="glass-panel animate-pulse rounded-3xl px-5 py-5" aria-hidden="true">
          <div class="h-3 w-24 rounded-full bg-canvas-100"></div>
          <div class="mt-4 h-10 w-36 rounded-full bg-canvas-100"></div>
          <div class="mt-3 h-3 w-full rounded-full bg-canvas-100"></div>
          <div class="mt-2 h-3 w-2/3 rounded-full bg-canvas-100"></div>
        </article>
      {/each}
    </div>
  {:else if storeMetrics}
    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
      <article class="glass-panel rounded-3xl px-5 py-5">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">Balance Toko</p>
        <p class="mt-3 font-display text-3xl font-bold text-ink-900">
          {formatCurrency(storeMetrics.balance_total)}
        </p>
        <p class="mt-2 text-sm leading-6 text-ink-700">
          Akumulasi saldo untuk {storeMetrics.accessible_store_count} toko dalam scope sesi ini.
        </p>
      </article>

      <article class="glass-panel rounded-3xl px-5 py-5">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">Pending QRIS</p>
        <p class="mt-3 font-display text-3xl font-bold text-ink-900">
          {storeMetrics.pending_qris_count}
        </p>
        <p class="mt-2 text-sm leading-6 text-ink-700">
          QRIS yang masih menunggu webhook atau reconcile provider.
        </p>
      </article>

      <article class="glass-panel rounded-3xl px-5 py-5">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
          Success Hari Ini
        </p>
        <p class="mt-3 font-display text-3xl font-bold text-ink-900">
          {storeMetrics.success_today_count}
        </p>
        <p class="mt-2 text-sm leading-6 text-ink-700">
          QRIS `store_topup` atau `member_payment` yang selesai hari ini.
        </p>
      </article>

      <article class="glass-panel rounded-3xl px-5 py-5">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
          Expired Hari Ini
        </p>
        <p class="mt-3 font-display text-3xl font-bold text-ink-900">
          {storeMetrics.expired_today_count}
        </p>
        <p class="mt-2 text-sm leading-6 text-ink-700">
          QRIS yang kedaluwarsa pada hari berjalan.
        </p>
      </article>

      <article class="glass-panel rounded-3xl px-5 py-5">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
          Income Bulan Ini
        </p>
        <p class="mt-3 font-display text-3xl font-bold text-ink-900">
          {formatCurrency(storeMetrics.monthly_store_income)}
        </p>
        <p class="mt-2 text-sm leading-6 text-ink-700">
          Kredit toko dari `member_payment.success` selama bulan berjalan.
        </p>
      </article>
    </div>
  {:else if platformMetrics}
    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      <article class="glass-panel rounded-3xl px-5 py-5">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
          Income Hari Ini
        </p>
        <p class="mt-3 font-display text-3xl font-bold text-ink-900">
          {formatCurrency(platformMetrics.platform_income_today)}
        </p>
        <p class="mt-2 text-sm leading-6 text-ink-700">
          Akumulasi fee platform dari `member_payment` dan withdraw sukses hari ini.
        </p>
      </article>

      <article class="glass-panel rounded-3xl px-5 py-5">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
          Income Bulan Ini
        </p>
        <p class="mt-3 font-display text-3xl font-bold text-ink-900">
          {formatCurrency(platformMetrics.platform_income_month)}
        </p>
        <p class="mt-2 text-sm leading-6 text-ink-700">
          Fee platform bulan berjalan untuk seluruh tenant.
        </p>
      </article>

      <article class="glass-panel rounded-3xl px-5 py-5">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">Total Toko</p>
        <p class="mt-3 font-display text-3xl font-bold text-ink-900">
          {platformMetrics.total_store_count}
        </p>
        <p class="mt-2 text-sm leading-6 text-ink-700">
          Semua toko aktif platform yang belum di-soft-delete.
        </p>
      </article>

      <article class="glass-panel rounded-3xl px-5 py-5">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
          Pending Withdraw
        </p>
        <p class="mt-3 font-display text-3xl font-bold text-ink-900">
          {platformMetrics.pending_withdraw_count}
        </p>
        <p class="mt-2 text-sm leading-6 text-ink-700">
          Withdrawal owner yang masih menunggu webhook atau status check.
        </p>
      </article>

      <article class="glass-panel rounded-3xl px-5 py-5">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
          Upstream Error 24h
        </p>
        <p class="mt-3 font-display text-3xl font-bold text-ink-900">
          {formatPercent(platformMetrics.upstream_error_rate_24h)}
        </p>
        <p class="mt-2 text-sm leading-6 text-ink-700">
          Rasio status check atau reconcile yang berakhir `upstream_error` 24 jam terakhir.
        </p>
      </article>

      <article class="glass-panel rounded-3xl px-5 py-5">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
          Callback Failure 24h
        </p>
        <p class="mt-3 font-display text-3xl font-bold text-ink-900">
          {formatPercent(platformMetrics.callback_failure_rate_24h)}
        </p>
        <p class="mt-2 text-sm leading-6 text-ink-700">
          Rasio attempt callback yang berstatus gagal dalam 24 jam terakhir.
        </p>
      </article>
    </div>
  {/if}
</section>
