<script lang="ts">
  import { onMount } from 'svelte';
  import type { ChartConfiguration } from 'chart.js';

  import { authSession } from '$lib/auth/client';
  import {
    chartGridColor as resolveChartGridColor,
    chartTextColor as resolveChartTextColor,
  } from '$lib/chart-theme';
  import EmptyState from '$lib/components/app/empty-state.svelte';
  import ChartCanvas from '$lib/components/app/chart-canvas.svelte';
  import GaugeRing from '$lib/components/app/gauge-ring.svelte';
  import MetricCard from '$lib/components/app/metric-card.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import { fetchDashboardCards, type DashboardPlatformMetrics, type DashboardStoreMetrics } from '$lib/dashboard/client';
  import { formatCurrency, formatDateTime, formatNumber, formatPercent, safeList } from '$lib/formatters';
  import { realtimeState } from '$lib/realtime/client';
  import { parseMoney } from '$lib/stores/client';
  import { resolvedTheme } from '$lib/theme';

  const relevantRealtimeEvents = new Set([
    'member_payment.success',
    'store_topup.success',
    'withdraw.success',
    'withdraw.failed',
    'callback.delivery_failed',
    'game.deposit.success',
    'game.withdraw.success',
    'store.low_balance',
  ]);

  const dashboardEventTypes = [
    'member_payment.success',
    'store_topup.success',
    'withdraw.success',
    'withdraw.failed',
    'callback.delivery_failed',
    'game.deposit.success',
    'game.withdraw.success',
    'store.low_balance',
  ];

  let loading = true;
  let errorMessage = '';
  let storeMetrics: DashboardStoreMetrics | null = null;
  let platformMetrics: DashboardPlatformMetrics | null = null;
  let lastSyncedAt: string | null = null;
  let requestInFlight = false;
  let lastRealtimeKey: string | null = null;
  let lastConnectionID: string | null = null;

  $: role = $authSession?.user.role ?? '';
  $: chartTextColor = resolveChartTextColor($resolvedTheme);
  $: chartGridColor = resolveChartGridColor($resolvedTheme);
  $: recentEvents = safeList($realtimeState.events)
    .filter((event) => relevantRealtimeEvents.has(event.type))
    .slice(0, 5);
  $: marqueeEvents = recentEvents.length > 0 ? [...recentEvents, ...recentEvents] : [];
  $: storeMixValues = storeMetrics
    ? [
        storeMetrics.pending_qris_count,
        storeMetrics.success_today_count,
        storeMetrics.expired_today_count,
      ]
    : [];
  $: storeMixTotal = storeMixValues.reduce((total, value) => total + value, 0);
  $: storeMixChart = buildDonutChart(
    ['Pending QRIS', 'Success Today', 'Expired Today'],
    storeMixValues,
    ['#22c977', '#efc86d', '#d66b5a'],
  );
  $: storeFinanceChart = buildBarChart(
    ['Balance Pool', 'Monthly Income'],
    [
      parseMoney(storeMetrics?.balance_total),
      parseMoney(storeMetrics?.monthly_store_income),
    ],
    ['#22c977', '#0f7242'],
  );
  $: storeRadarChart = buildRadarChart(
    ['Accessible', 'Active', 'Low Balance', 'Pending', 'Success', 'Expired'],
    [
      storeMetrics?.accessible_store_count ?? 0,
      storeMetrics?.active_store_count ?? 0,
      storeMetrics?.low_balance_store_count ?? 0,
      storeMetrics?.pending_qris_count ?? 0,
      storeMetrics?.success_today_count ?? 0,
      storeMetrics?.expired_today_count ?? 0,
    ],
    '#22c977',
    'rgba(34, 201, 119, 0.16)',
  );
  $: liveEventChart = buildHorizontalBarChart(
    dashboardEventTypes.map((type) => type.replace('.', ' ')),
    dashboardEventTypes.map(
      (type) => recentEvents.filter((event) => event.type === type).length,
    ),
    '#d7a236',
  );
  $: platformFinanceChart = buildBarChart(
    ['Today', 'Month'],
    [
      parseMoney(platformMetrics?.platform_income_today),
      parseMoney(platformMetrics?.platform_income_month),
    ],
    ['#efc86d', '#22c977'],
  );
  $: platformOpsChart = buildDonutChart(
    ['Pending Withdraw', 'Active Store', 'At Risk'],
    [
      platformMetrics?.pending_withdraw_count ?? 0,
      platformMetrics?.active_store_count ?? 0,
      platformMetrics?.low_balance_store_count ?? 0,
    ],
    ['#efc86d', '#22c977', '#d66b5a'],
  );
  $: platformRadarChart = buildRadarChart(
    ['Total Stores', 'Active', 'Low Balance', 'Pending Withdraw', 'Upstream Error', 'Callback Failure'],
    [
      platformMetrics?.total_store_count ?? 0,
      platformMetrics?.active_store_count ?? 0,
      platformMetrics?.low_balance_store_count ?? 0,
      platformMetrics?.pending_withdraw_count ?? 0,
      platformMetrics?.upstream_error_rate_24h ?? 0,
      platformMetrics?.callback_failure_rate_24h ?? 0,
    ],
    '#efc86d',
    'rgba(239, 200, 109, 0.16)',
  );
  $: showOwnerOnboarding = role === 'owner' && (storeMetrics?.accessible_store_count ?? 0) === 0;
  $: showPlatformOnboarding =
    (role === 'dev' || role === 'superadmin') &&
    (platformMetrics?.total_store_count ?? 0) === 0;

  onMount(() => {
    let active = true;

    async function loadCards() {
      if (!active || requestInFlight) {
        return;
      }

      requestInFlight = true;
      errorMessage = '';

      const dashboardResponse = await fetchDashboardCards();
      requestInFlight = false;

      if (!active) {
        return;
      }

      if (!dashboardResponse.status || dashboardResponse.message !== 'SUCCESS') {
        errorMessage = dashboardResponse.message;
        loading = false;
        return;
      }

      storeMetrics = dashboardResponse.data.store_metrics ?? null;
      platformMetrics = dashboardResponse.data.platform_metrics ?? null;
      lastSyncedAt = new Date().toISOString();
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

  function liveMode(status: string) {
    return status === 'connected' ? 'Realtime aktif' : 'Fallback polling 30s';
  }

  function buildDonutChart(
    labels: string[],
    values: number[],
    colors: string[],
  ): ChartConfiguration<'doughnut'> {
    return {
      type: 'doughnut',
      data: {
        labels,
        datasets: [
          {
            data: values.map((value) => Math.max(0, value)),
            backgroundColor: colors,
            borderWidth: 0,
            hoverOffset: 6,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        cutout: '72%',
        plugins: {
          legend: {
            position: 'bottom',
            labels: {
              color: chartTextColor,
              usePointStyle: true,
              boxWidth: 10,
              padding: 18,
            },
          },
          tooltip: {
            callbacks: {
              label: (context) =>
                `${context.label}: ${formatNumber(Number(context.parsed ?? 0))}`,
            },
          },
        },
      },
    };
  }

  function buildBarChart(
    labels: string[],
    values: number[],
    colors: string[],
  ): ChartConfiguration<'bar'> {
    return {
      type: 'bar',
      data: {
        labels,
        datasets: [
          {
            data: values.map((value) => Math.max(0, value)),
            backgroundColor: colors,
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
          tooltip: {
            callbacks: {
              label: (context) => formatCurrency(Number(context.parsed.y ?? 0)),
            },
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
              callback: (value) => formatCurrency(Number(value), { compact: true }),
            },
            grid: {
              color: chartGridColor,
            },
          },
        },
      },
    };
  }

  function buildHorizontalBarChart(
    labels: string[],
    values: number[],
    color: string,
  ): ChartConfiguration<'bar'> {
    return {
      type: 'bar',
      data: {
        labels,
        datasets: [
          {
            data: values.map((value) => Math.max(0, value)),
            backgroundColor: color,
            borderRadius: 999,
            borderSkipped: false,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        indexAxis: 'y',
        plugins: {
          legend: {
            display: false,
          },
          tooltip: {
            callbacks: {
              label: (context) => formatNumber(Number(context.parsed.x ?? 0)),
            },
          },
        },
        scales: {
          x: {
            ticks: {
              color: chartTextColor,
              precision: 0,
            },
            grid: {
              color: chartGridColor,
            },
          },
          y: {
            ticks: {
              color: chartTextColor,
            },
            grid: {
              display: false,
            },
          },
        },
      },
    };
  }

  function buildRadarChart(
    labels: string[],
    values: number[],
    borderColor: string,
    backgroundColor: string,
  ): ChartConfiguration<'radar'> {
    return {
      type: 'radar',
      data: {
        labels,
        datasets: [
          {
            label: 'Signal',
            data: values.map((value) => Math.max(0, value)),
            borderColor,
            backgroundColor,
            pointBackgroundColor: borderColor,
            pointRadius: 3,
            borderWidth: 2,
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
          tooltip: {
            callbacks: {
              label: (context) => formatNumber(Number(context.parsed.r ?? 0)),
            },
          },
        },
        scales: {
          r: {
            angleLines: {
              color: chartGridColor,
            },
            grid: {
              color: chartGridColor,
            },
            pointLabels: {
              color: chartTextColor,
              font: {
                size: 11,
              },
            },
            ticks: {
              color: chartTextColor,
              backdropColor: 'transparent',
              precision: 0,
            },
          },
        },
      },
    };
  }
</script>

<svelte:head>
  <title>Dashboard | onixggr</title>
</svelte:head>

<section class="space-y-6">
  <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
    <div class="grid gap-6 xl:grid-cols-[1.18fr_0.82fr]">
      <div class="space-y-4">
        <p class="status-chip w-fit">Dashboard command</p>
        <div class="space-y-3">
          <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
            Role-aware signal wall untuk flow uang, store health, dan live operations.
          </h1>
          <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
            Ringkasan ini membaca aggregate backend sesuai scope role, lalu memperkaya tampilan
            dengan chart, store health, dan event rail tanpa membuat asumsi data baru di frontend.
          </p>
        </div>
      </div>

      <div class="grid gap-3 sm:grid-cols-2">
        <article class="rounded-[1.8rem] border border-white/12 bg-white/7 p-5 backdrop-blur">
          <p class="text-[0.68rem] font-semibold uppercase tracking-[0.28em] text-white/45">
            Session
          </p>
          <p class="mt-3 text-lg font-semibold text-white">{$authSession?.user.email ?? '-'}</p>
          <p class="mt-1 text-sm text-white/62">Role {$authSession?.user.role ?? '-'}</p>
        </article>

        <article class="rounded-[1.8rem] border border-white/12 bg-white/7 p-5 backdrop-blur">
          <p class="text-[0.68rem] font-semibold uppercase tracking-[0.28em] text-white/45">
            Sync Mode
          </p>
          <p class="mt-3 text-lg font-semibold text-white">{liveMode($realtimeState.status)}</p>
          <p class="mt-1 text-sm text-white/62">
            Last sync {lastSyncedAt ? formatDateTime(lastSyncedAt) : 'belum ada'}
          </p>
        </article>
      </div>
    </div>
  </section>

  {#if errorMessage !== ''}
    <Notice tone="error" title="Dashboard Unavailable" message={`Gagal memuat dashboard cards: ${errorMessage}`} />
  {/if}

  {#if loading}
    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      {#each Array(8) as _}
        <article class="glass-panel animate-pulse rounded-[1.8rem] px-5 py-5" aria-hidden="true">
          <div class="h-3 w-24 rounded-full bg-canvas-100"></div>
          <div class="mt-4 h-10 w-36 rounded-full bg-canvas-100"></div>
          <div class="mt-3 h-3 w-full rounded-full bg-canvas-100"></div>
        </article>
      {/each}
    </div>
  {:else if storeMetrics}
    {#if showOwnerOnboarding}
      <section class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
        <div class="grid gap-4 xl:grid-cols-[1.05fr_0.95fr]">
          <div class="space-y-3">
            <p class="section-kicker !text-brand-700">Owner launch path</p>
            <h2 class="font-display text-3xl font-bold tracking-tight text-ink-900">
              Belum ada store. Langkah berikutnya adalah membuat tenant pertama.
            </h2>
            <p class="text-sm leading-7 text-ink-700">
              Karena Anda sudah login sebagai owner, tahap berikutnya adalah membuka halaman Stores
              untuk membuat store pertama. Saat store dibuat, token awal akan diterbitkan satu kali,
              lalu Anda bisa lanjut ke API Docs untuk integrasi website.
            </p>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <a class="glass-panel rounded-[1.6rem] px-4 py-4 text-sm text-ink-700" href="/app/onboarding">
              <p class="font-semibold text-ink-900">1. Open onboarding</p>
              <p class="mt-2 leading-6">
                Gunakan runway owner untuk membuat tenant pertama dan dapatkan one-time API token.
              </p>
            </a>
            <a class="glass-panel rounded-[1.6rem] px-4 py-4 text-sm text-ink-700" href="/app/api-docs">
              <p class="font-semibold text-ink-900">2. Integrate website</p>
              <p class="mt-2 leading-6">
                Lanjut ke callback, store API, dan contoh request/response untuk owner.
              </p>
            </a>
          </div>
        </div>
      </section>
    {/if}

    <section class="dashboard-chart-card dashboard-chart-card--dark surface-grid">
      <div class="grid gap-6 xl:grid-cols-[minmax(0,1.12fr)_minmax(18rem,22rem)]">
        <div class="space-y-5">
          <div class="space-y-3">
            <p class="section-kicker">Store Matrix</p>
            <h2 class="font-display text-3xl font-bold tracking-tight sm:text-4xl">
              Matrix lantai transaksi untuk scope store yang sedang aktif.
            </h2>
            <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
              Dashboard store menonjolkan komposisi pending, success, expired, balance pool, dan
              tekanan low balance tanpa menebak data baru di frontend.
            </p>
          </div>

          {#if marqueeEvents.length > 0}
            <div class="event-marquee">
              <div class="event-marquee__track">
                {#each marqueeEvents as event}
                  <span class="event-marquee__pill">
                    <span>{event.type}</span>
                    <span>{event.channel}</span>
                  </span>
                {/each}
              </div>
            </div>
          {/if}

          <div class="grid gap-4 lg:grid-cols-[1fr_1fr]">
            <article class="rounded-[1.7rem] border border-white/10 bg-white/6 p-5 backdrop-blur">
              <p class="text-[0.72rem] font-semibold uppercase tracking-[0.28em] text-white/46">
                Capital pool
              </p>
              <p class="mt-4 font-display text-4xl font-semibold tracking-tight text-white">
                {formatCurrency(storeMetrics.balance_total)}
              </p>
              <p class="mt-3 text-sm leading-6 text-white/66">
                Live balance pool untuk store yang bisa diakses role saat ini.
              </p>
            </article>

            <article class="rounded-[1.7rem] border border-white/10 bg-white/6 p-5 backdrop-blur">
              <p class="text-[0.72rem] font-semibold uppercase tracking-[0.28em] text-white/46">
                Monthly intake
              </p>
              <p class="mt-4 font-display text-4xl font-semibold tracking-tight text-white">
                {formatCurrency(storeMetrics.monthly_store_income)}
              </p>
              <p class="mt-3 text-sm leading-6 text-white/66">
                Inflow bulan berjalan dari member payment yang sudah final.
              </p>
            </article>
          </div>
        </div>

        <article class="rounded-[1.9rem] border border-white/10 bg-white/6 p-5 backdrop-blur">
          <p class="text-[0.72rem] font-semibold uppercase tracking-[0.28em] text-white/46">
            Operations radar
          </p>
          <h3 class="mt-3 font-display text-2xl font-semibold tracking-tight text-white">
            Store pressure map
          </h3>
          <p class="mt-2 text-sm leading-6 text-white/66">
            Accessible stores, active stores, low-balance pressure, QRIS queue, dan outcome hari ini.
          </p>
          <ChartCanvas class="mt-5 h-[320px]" config={storeRadarChart} />
        </article>
      </div>
    </section>

    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-6">
      <MetricCard
        eyebrow="Store Balance"
        title="Balance pool"
        value={formatCurrency(storeMetrics.balance_total)}
        detail={`Akumulasi saldo untuk ${formatNumber(storeMetrics.accessible_store_count)} toko yang bisa diakses sesi ini.`}
        tone="brand"
      />
      <MetricCard
        eyebrow="Store Health"
        title="Active stores"
        value={formatNumber(storeMetrics.active_store_count)}
        detail={`Low balance terdeteksi pada ${formatNumber(storeMetrics.low_balance_store_count)} toko dalam scope sesi.`}
      />
      <MetricCard
        eyebrow="QRIS Queue"
        title="Pending QRIS"
        value={formatNumber(storeMetrics.pending_qris_count)}
        detail="Menunggu webhook provider atau QRIS reconcile worker."
        tone="accent"
      />
      <MetricCard
        eyebrow="Today"
        title="Success today"
        value={formatNumber(storeMetrics.success_today_count)}
        detail="store_topup atau member_payment yang selesai hari ini."
      />
      <MetricCard
        eyebrow="Today"
        title="Expired today"
        value={formatNumber(storeMetrics.expired_today_count)}
        detail="QRIS yang kedaluwarsa pada hari berjalan."
        tone="danger"
      />
      <MetricCard
        eyebrow="Monthly"
        title="Store income"
        value={formatCurrency(storeMetrics.monthly_store_income)}
        detail="Kredit toko dari member_payment.success selama bulan berjalan."
      />
    </div>

    <div class="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
      <section class="dashboard-chart-card rounded-[2.2rem] p-5 sm:p-6">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p class="section-kicker !text-brand-700">Signal mix</p>
            <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
              Transaction state composition
            </h2>
          </div>
          <span class="surface-chip">{formatNumber(storeMixTotal)} tracked event</span>
        </div>

        {#if storeMixTotal > 0}
          <div class="mt-6 grid gap-6 lg:grid-cols-[0.95fr_1.05fr]">
            <ChartCanvas class="h-[320px]" config={storeMixChart} />
            <div class="grid gap-4">
              <article class="rounded-[1.7rem] bg-canvas-50 px-5 py-5">
                <p class="text-sm font-semibold text-ink-900">Balance vs income</p>
                <p class="mt-2 text-sm leading-6 text-ink-700">
                  Compare current accessible store balance against monthly credit inflow from
                  member payment.
                </p>
                <ChartCanvas class="mt-4 h-[220px]" config={storeFinanceChart} />
              </article>

              <article class="rounded-[1.7rem] border border-ink-100 bg-white/80 px-5 py-5">
                <p class="text-sm font-semibold text-ink-900">Store health</p>
                <p class="mt-2 text-sm leading-6 text-ink-700">
                  {storeMetrics.low_balance_store_count === 0
                    ? 'Tidak ada store yang berada di bawah threshold saat ini.'
                    : `${formatNumber(storeMetrics.low_balance_store_count)} store berada di threshold atau di bawah threshold.`}
                </p>
                <div class="mt-4 space-y-3">
                  {#if storeMetrics.low_balance_store_count === 0}
                    <div class="rounded-[1.4rem] bg-brand-100/70 px-4 py-4 text-sm text-brand-700">
                      Semua store dalam scope ini masih berada di atas threshold.
                    </div>
                  {:else}
                    <div class="rounded-[1.4rem] border border-amber-200/60 bg-linear-to-r from-accent-100/40 to-white px-4 py-4 text-sm leading-6 text-ink-700">
                      Low balance detail kini dihitung langsung oleh backend supaya dashboard tetap
                      ringan walau roster store sudah besar.
                    </div>
                  {/if}
                </div>
              </article>
            </div>
          </div>
        {:else}
          <div class="mt-6">
            <EmptyState
              eyebrow="Signal Mix"
              title="Belum ada event transaksi untuk divisualisasikan"
              body="Saat pending, success, atau expired QRIS mulai bergerak, chart komposisi di dashboard akan terisi otomatis."
            />
          </div>
        {/if}
      </section>

      <section class="space-y-6">
        <div class="dashboard-chart-card rounded-[2.2rem] p-5 sm:p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Flow intensity</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Live event density
              </h2>
            </div>
            <span class="surface-chip">{formatNumber(recentEvents.length)} event</span>
          </div>

          <ChartCanvas class="mt-6 h-[310px]" config={liveEventChart} />
        </div>

        <div class="dashboard-chart-card rounded-[2.2rem] p-5 sm:p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Realtime rail</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Latest transaction events
              </h2>
            </div>
            <span class="surface-chip">{$realtimeState.status}</span>
          </div>

          <div class="mt-6 space-y-3">
            {#if recentEvents.length === 0}
              <div class="rounded-[1.6rem] bg-canvas-50 px-4 py-4 text-sm leading-6 text-ink-700">
                Belum ada event realtime yang masuk ke sesi ini.
              </div>
            {:else}
              {#each recentEvents as event}
                <article class="rounded-[1.6rem] border border-ink-100 bg-white/78 px-4 py-4 transition duration-200 hover:-translate-y-0.5 hover:shadow-[0_18px_36px_rgba(22,38,31,0.08)]">
                  <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div>
                      <p class="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-brand-700">
                        {event.type}
                      </p>
                      <p class="mt-2 text-sm font-semibold text-ink-900">{event.channel}</p>
                    </div>
                    <p class="text-xs text-ink-500">{formatDateTime(event.created_at)}</p>
                  </div>
                </article>
              {/each}
            {/if}
          </div>
        </div>

        <div class="dashboard-chart-card rounded-[2.2rem] p-5 sm:p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Scope telemetry</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Portfolio snapshot
              </h2>
            </div>
            <span class="surface-chip">{formatNumber(storeMetrics.accessible_store_count)} store</span>
          </div>

          <div class="mt-6 grid gap-4 sm:grid-cols-2">
            <MetricCard
              class="h-full"
              eyebrow="Store Count"
              title="Accessible stores"
              value={formatNumber(storeMetrics.accessible_store_count)}
              detail="Jumlah toko yang bisa dibaca oleh role sesi aktif."
              tone="brand"
            />
            <MetricCard
              class="h-full"
              eyebrow="Threshold"
              title="Low balance"
              value={formatNumber(storeMetrics.low_balance_store_count)}
              detail="Store yang sudah masuk alert threshold berdasarkan saldo current balance."
              tone={storeMetrics.low_balance_store_count > 0 ? 'accent' : 'default'}
            />
          </div>
        </div>
      </section>
    </div>
  {:else if platformMetrics}
    {#if showPlatformOnboarding}
      <section class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
        <div class="grid gap-4 xl:grid-cols-[1.05fr_0.95fr]">
          <div class="space-y-3">
            <p class="section-kicker !text-brand-700">Platform onboarding</p>
            <h2 class="font-display text-3xl font-bold tracking-tight text-ink-900">
              Belum ada tenant aktif. Provision owner dulu.
            </h2>
            <p class="text-sm leading-7 text-ink-700">
              Flow yang benar sekarang sudah tersedia: buat akun owner dari halaman Users, owner
              login, lalu owner membuat store dan melanjutkan integrasi dari API Docs.
            </p>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <a class="glass-panel rounded-[1.6rem] px-4 py-4 text-sm text-ink-700" href="/app/onboarding">
              <p class="font-semibold text-ink-900">Open Onboarding</p>
              <p class="mt-2 leading-6">
                Provision owner dan kelola tenant runway dari surface yang paling ringkas.
              </p>
            </a>
            <a class="glass-panel rounded-[1.6rem] px-4 py-4 text-sm text-ink-700" href="/app/users">
              <p class="font-semibold text-ink-900">Open Users</p>
              <p class="mt-2 leading-6">
                Buka control plane user jika perlu reaktivasi owner atau provision role platform.
              </p>
            </a>
          </div>
        </div>
      </section>
    {/if}

    <section class="dashboard-chart-card dashboard-chart-card--dark surface-grid">
      <div class="grid gap-6 xl:grid-cols-[minmax(0,1.12fr)_minmax(18rem,22rem)]">
        <div class="space-y-5">
          <div class="space-y-3">
            <p class="section-kicker">Platform Matrix</p>
            <h2 class="font-display text-3xl font-bold tracking-tight sm:text-4xl">
              Telemetry platform untuk tenant, callback queue, dan health pressure.
            </h2>
            <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
              View ini menaruh platform fee, withdraw queue, risk rate, dan tenant pressure dalam
              satu frame supaya role platform cepat membaca incident surface.
            </p>
          </div>

          {#if marqueeEvents.length > 0}
            <div class="event-marquee">
              <div class="event-marquee__track">
                {#each marqueeEvents as event}
                  <span class="event-marquee__pill">
                    <span>{event.type}</span>
                    <span>{event.channel}</span>
                  </span>
                {/each}
              </div>
            </div>
          {/if}

          <div class="grid gap-4 lg:grid-cols-[1fr_1fr]">
            <article class="rounded-[1.7rem] border border-white/10 bg-white/6 p-5 backdrop-blur">
              <p class="text-[0.72rem] font-semibold uppercase tracking-[0.28em] text-white/46">
                Fee today
              </p>
              <p class="mt-4 font-display text-4xl font-semibold tracking-tight text-white">
                {formatCurrency(platformMetrics.platform_income_today)}
              </p>
              <p class="mt-3 text-sm leading-6 text-white/66">
                Akumulasi fee platform dari flow final hari ini.
              </p>
            </article>

            <article class="rounded-[1.7rem] border border-white/10 bg-white/6 p-5 backdrop-blur">
              <p class="text-[0.72rem] font-semibold uppercase tracking-[0.28em] text-white/46">
                Fee month
              </p>
              <p class="mt-4 font-display text-4xl font-semibold tracking-tight text-white">
                {formatCurrency(platformMetrics.platform_income_month)}
              </p>
              <p class="mt-3 text-sm leading-6 text-white/66">
                Akumulasi fee platform bulan berjalan.
              </p>
            </article>
          </div>
        </div>

        <article class="rounded-[1.9rem] border border-white/10 bg-white/6 p-5 backdrop-blur">
          <p class="text-[0.72rem] font-semibold uppercase tracking-[0.28em] text-white/46">
            Platform radar
          </p>
          <h3 class="mt-3 font-display text-2xl font-semibold tracking-tight text-white">
            Risk pressure map
          </h3>
          <p class="mt-2 text-sm leading-6 text-white/66">
            Total stores, active stores, low balance, pending withdraw, dan error rate 24 jam.
          </p>
          <ChartCanvas class="mt-5 h-[320px]" config={platformRadarChart} />
        </article>
      </div>
    </section>

    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
      <MetricCard
        eyebrow="Platform Fee"
        title="Income today"
        value={formatCurrency(platformMetrics.platform_income_today)}
        detail="Akumulasi fee platform dari flow sukses hari ini."
        tone="accent"
      />
      <MetricCard
        eyebrow="Platform Fee"
        title="Income month"
        value={formatCurrency(platformMetrics.platform_income_month)}
        detail="Akumulasi fee platform bulan berjalan."
        tone="brand"
      />
      <MetricCard
        eyebrow="Tenant"
        title="Total store"
        value={formatNumber(platformMetrics.total_store_count)}
        detail="Store aktif platform yang belum di-soft-delete."
      />
      <MetricCard
        eyebrow="Health"
        title="Low balance"
        value={formatNumber(platformMetrics.low_balance_store_count)}
        detail={`Dari ${formatNumber(platformMetrics.active_store_count)} toko active yang sedang berjalan.`}
        tone={platformMetrics.low_balance_store_count > 0 ? 'accent' : 'default'}
      />
      <MetricCard
        eyebrow="Queue"
        title="Pending withdraw"
        value={formatNumber(platformMetrics.pending_withdraw_count)}
        detail="Masih menunggu webhook transfer atau status checker."
      />
    </div>

    <div class="grid gap-6 xl:grid-cols-[1.08fr_0.92fr]">
      <section class="dashboard-chart-card rounded-[2.2rem] p-5 sm:p-6">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p class="section-kicker !text-brand-700">Platform finance</p>
            <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
              Income and operating pressure
            </h2>
          </div>
          <span class="surface-chip">{formatNumber(platformMetrics.total_store_count)} tenant</span>
        </div>

        <div class="mt-6 grid gap-6 lg:grid-cols-[1fr_1fr]">
          <article class="rounded-[1.7rem] bg-canvas-50 px-5 py-5">
            <p class="text-sm font-semibold text-ink-900">Income ladder</p>
            <p class="mt-2 text-sm leading-6 text-ink-700">
              Compare platform fee accumulation today versus month-to-date.
            </p>
            <ChartCanvas class="mt-4 h-[260px]" config={platformFinanceChart} />
          </article>

          <article class="rounded-[1.7rem] bg-canvas-50 px-5 py-5">
            <p class="text-sm font-semibold text-ink-900">Operations mix</p>
            <p class="mt-2 text-sm leading-6 text-ink-700">
              Pending withdraw volume versus active tenant count and current low-balance risk.
            </p>
            <ChartCanvas class="mt-4 h-[260px]" config={platformOpsChart} />
          </article>
        </div>
      </section>

      <section class="space-y-6">
        <div class="grid gap-4 sm:grid-cols-2">
          <GaugeRing
            label="Upstream error 24h"
            value={platformMetrics.upstream_error_rate_24h}
            detail="Status check atau reconcile yang berakhir upstream_error dalam 24 jam terakhir."
            tone={platformMetrics.upstream_error_rate_24h >= 5 ? 'accent' : 'brand'}
          />
          <GaugeRing
            label="Callback failure 24h"
            value={platformMetrics.callback_failure_rate_24h}
            detail="Callback owner yang gagal setelah retry worker selama 24 jam terakhir."
            tone={platformMetrics.callback_failure_rate_24h >= 5 ? 'accent' : 'slate'}
          />
        </div>

        <div class="dashboard-chart-card rounded-[2.2rem] p-5 sm:p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Event pressure</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Recent event density
              </h2>
            </div>
            <span class="surface-chip">{formatNumber(recentEvents.length)} event</span>
          </div>

          <ChartCanvas class="mt-6 h-[300px]" config={liveEventChart} />
        </div>

        <div class="dashboard-chart-card rounded-[2.2rem] p-5 sm:p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Live operations</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Realtime feed
              </h2>
            </div>
            <span class="surface-chip">{$realtimeState.status}</span>
          </div>

          <div class="mt-6 space-y-3">
            {#if recentEvents.length === 0}
              <div class="rounded-[1.6rem] bg-canvas-50 px-4 py-4 text-sm leading-6 text-ink-700">
                Belum ada event realtime lintas store pada sesi ini.
              </div>
            {:else}
              {#each recentEvents as event}
                <article class="rounded-[1.6rem] border border-ink-100 bg-white/78 px-4 py-4">
                  <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div>
                      <p class="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-brand-700">
                        {event.type}
                      </p>
                      <p class="mt-2 text-sm font-semibold text-ink-900">{event.channel}</p>
                    </div>
                    <p class="text-xs text-ink-500">{formatDateTime(event.created_at)}</p>
                  </div>
                </article>
              {/each}
            {/if}
          </div>
        </div>
      </section>
    </div>
  {:else}
    <EmptyState
      eyebrow="Dashboard Scope"
      title="Belum ada kartu yang bisa dirender"
      body="Backend belum mengembalikan kartu store maupun platform untuk sesi ini. Coba refresh setelah scope role atau data toko berubah."
    />
  {/if}
</section>
