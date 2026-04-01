<script lang="ts">
  import { get } from 'svelte/store';
  import { onMount } from 'svelte';
  import type { ChartConfiguration } from 'chart.js';

  import { authSession } from '$lib/auth/client';
  import {
    chartGridColor as resolveChartGridColor,
    chartTextColor as resolveChartTextColor,
  } from '$lib/chart-theme';
  import type {
    CallbackAttemptRecord,
    CallbackQueueItem,
    CallbackQueueSummary,
    CallbackStatus,
  } from '$lib/callbacks/client';
  import {
    fetchCallbackAttempts,
    fetchCallbackQueue,
  } from '$lib/callbacks/client';
  import ChartCanvas from '$lib/components/app/chart-canvas.svelte';
  import DateRangeFilter from '$lib/components/app/date-range-filter.svelte';
  import EmptyState from '$lib/components/app/empty-state.svelte';
  import ExportActions from '$lib/components/app/export-actions.svelte';
  import GaugeRing from '$lib/components/app/gauge-ring.svelte';
  import MetricCard from '$lib/components/app/metric-card.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import PaginationControls from '$lib/components/app/pagination-controls.svelte';
  import StoreScopePicker from '$lib/components/app/store-scope-picker.svelte';
  import Button from '$lib/components/ui/button/button.svelte';
  import {
    exportRowsToCSV,
    exportRowsToPDF,
    exportRowsToXLSX,
  } from '$lib/exporters';
  import {
    formatDateTime,
    formatNumber,
    formatPercent,
    formatRelativeShort,
  } from '$lib/formatters';
  import type { HealthReport } from '$lib/ops/client';
  import { fetchLiveHealth, fetchReadyHealth } from '$lib/ops/client';
  import type { Store } from '$lib/stores/client';
  import {
    hydratePreferredStoreID,
    preferredStoreID,
    setPreferredStoreID,
  } from '$lib/stores/preferences';
  import { resolvedTheme } from '$lib/theme';

  const emptySummary: CallbackQueueSummary = {
    total_count: 0,
    pending_count: 0,
    retrying_count: 0,
    success_count: 0,
    failed_count: 0,
  };

  type HealthCard = {
    eyebrow: string;
    title: string;
    value: string;
    detail: string;
    tone: 'brand' | 'accent' | 'default' | 'danger';
  };

  let mounted = false;
  let loading = true;
  let refreshing = false;
  let errorMessage = '';
  let queueItems: CallbackQueueItem[] = [];
  let queueSummary: CallbackQueueSummary = { ...emptySummary };
  let page = 1;
  let pageSize = 12;
  let lastQueueKey = '';

  let searchTerm = '';
  let statusFilter: 'all' | CallbackStatus = 'all';
  let createdFrom = '';
  let createdTo = '';
  let appliedSearchTerm = '';
  let appliedStatusFilter: 'all' | CallbackStatus = 'all';
  let appliedCreatedFrom = '';
  let appliedCreatedTo = '';

  let selectedStoreID = '';
  let selectedStore: Store | null = null;
  let storeScopeLoading = true;
  let storeScopeTotalCount = 0;

  let selectedCallbackID = '';
  let selectedCallback: CallbackQueueItem | null = null;
  let attemptsLoading = false;
  let attemptsRefreshing = false;
  let attemptsError = '';
  let attemptsPage = 1;
  let attemptsPageSize = 6;
  let attemptsTotalCount = 0;
  let attempts: CallbackAttemptRecord[] = [];
  let lastAttemptsKey = '';

  let liveHealth: HealthReport | null = null;
  let readyHealth: HealthReport | null = null;
  let healthLoading = true;

  $: role = $authSession?.user.role ?? '';
  $: isPlatformRole = role === 'dev' || role === 'superadmin';
  $: liveStatusLabel = liveHealth?.status ?? 'loading';
  $: readyStatusLabel = readyHealth?.status ?? 'loading';
  $: dependencyCounts = summarizeDependencies(readyHealth);
  $: terminalCount = queueSummary.success_count + queueSummary.failed_count;
  $: terminalSuccessRate =
    terminalCount === 0 ? 100 : (queueSummary.success_count / terminalCount) * 100;
  $: attentionLoad =
    queueSummary.total_count === 0
      ? 0
      : ((queueSummary.pending_count +
          queueSummary.retrying_count +
          queueSummary.failed_count) /
          queueSummary.total_count) *
        100;
  $: chartTextColor = resolveChartTextColor($resolvedTheme);
  $: chartGridColor = resolveChartGridColor($resolvedTheme);
  $: queueChart = buildQueueChart(queueSummary);
  $: dependencyChart = buildDependencyChart(readyHealth);
  $: healthCards = [
    {
      eyebrow: 'Health',
      title: 'Live endpoint',
      value: liveStatusLabel.toUpperCase(),
      detail:
        'Probe public yang memastikan proses API hidup dan origin tunnel merespons tanpa auth.',
      tone: liveHealth?.status === 'ok' ? 'brand' : 'danger',
    },
    {
      eyebrow: 'Readiness',
      title: 'Ready gate',
      value: readyStatusLabel.toUpperCase(),
      detail:
        'Probe dependency untuk Postgres, Redis, dan upstream degraded state yang dipakai sebelum traffic real.',
      tone:
        readyHealth?.status === 'ready'
          ? 'brand'
          : readyHealth?.status === 'degraded'
            ? 'accent'
            : 'danger',
    },
    {
      eyebrow: 'Callback Queue',
      title: 'Active backlog',
      value: formatNumber(queueSummary.pending_count + queueSummary.retrying_count),
      detail:
        'Pending + retrying callback yang masih perlu delivery atau observability follow-up.',
      tone:
        queueSummary.retrying_count > 0 || queueSummary.failed_count > 0
          ? 'accent'
          : 'default',
    },
    {
      eyebrow: 'Failures',
      title: 'Terminal failed',
      value: formatNumber(queueSummary.failed_count),
      detail:
        'Callback yang sudah berhenti retry. Event ini juga masuk notification stream dev dan superadmin.',
      tone: queueSummary.failed_count > 0 ? 'danger' : 'default',
    },
  ] satisfies HealthCard[];
  onMount(() => {
    mounted = true;
    hydratePreferredStoreID();
    selectedStoreID = get(preferredStoreID);

    const interval = window.setInterval(() => {
      if (!mounted || !isPlatformRole) {
        return;
      }

      void refreshAll(true);
    }, 15000);

    void refreshAll();

    return () => {
      mounted = false;
      window.clearInterval(interval);
    };
  });

  $: if (mounted && isPlatformRole) {
    const nextQueueKey = [
      selectedStoreID,
      appliedSearchTerm,
      appliedStatusFilter,
      appliedCreatedFrom,
      appliedCreatedTo,
      page,
      pageSize,
    ].join(':');

    if (nextQueueKey !== lastQueueKey) {
      lastQueueKey = nextQueueKey;
      void loadQueue();
    }
  }

  $: if (mounted && isPlatformRole) {
    const nextAttemptsKey = [selectedCallbackID, attemptsPage, attemptsPageSize].join(':');
    if (nextAttemptsKey !== lastAttemptsKey) {
      lastAttemptsKey = nextAttemptsKey;
      void loadAttempts();
    }
  }

  async function refreshAll(background = false) {
    await Promise.all([loadQueue(background), loadHealth(background)]);
  }

  async function loadQueue(background = false) {
    if (!isPlatformRole) {
      queueItems = [];
      queueSummary = { ...emptySummary };
      loading = false;
      refreshing = false;
      return;
    }

    if (background) {
      refreshing = true;
    } else {
      loading = true;
    }

    const response = await fetchCallbackQueue({
      query: appliedSearchTerm,
      status: appliedStatusFilter,
      storeID: selectedStoreID,
      createdFrom: appliedCreatedFrom,
      createdTo: appliedCreatedTo,
      limit: pageSize,
      offset: (page - 1) * pageSize,
    });

    if (!mounted) {
      return;
    }

    refreshing = false;
    loading = false;

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = response.message;
      queueItems = [];
      queueSummary = { ...emptySummary };
      return;
    }

    errorMessage = '';
    queueItems = response.data.items ?? [];
    queueSummary = response.data.summary ?? { ...emptySummary };

    const matched =
      queueItems.find((item) => item.id === selectedCallbackID) ?? null;
    if (matched) {
      selectedCallback = matched;
    } else if (queueItems.length > 0) {
      selectedCallbackID = queueItems[0].id;
      selectedCallback = queueItems[0];
      attemptsPage = 1;
      lastAttemptsKey = '';
    } else {
      selectedCallbackID = '';
      selectedCallback = null;
      attempts = [];
      attemptsTotalCount = 0;
      attemptsError = '';
    }
  }

  async function loadAttempts(background = false) {
    if (!isPlatformRole || selectedCallbackID === '') {
      attempts = [];
      attemptsTotalCount = 0;
      attemptsLoading = false;
      attemptsRefreshing = false;
      attemptsError = '';
      return;
    }

    if (background) {
      attemptsRefreshing = true;
    } else {
      attemptsLoading = true;
    }

    const response = await fetchCallbackAttempts(selectedCallbackID, {
      limit: attemptsPageSize,
      offset: (attemptsPage - 1) * attemptsPageSize,
    });

    if (!mounted) {
      return;
    }

    attemptsLoading = false;
    attemptsRefreshing = false;

    if (!response.status || response.message !== 'SUCCESS') {
      attemptsError = response.message;
      attempts = [];
      attemptsTotalCount = 0;
      return;
    }

    attemptsError = '';
    attempts = response.data.items ?? [];
    attemptsTotalCount = response.data.total_count ?? 0;
  }

  async function loadHealth(background = false) {
    if (background) {
      healthLoading = true;
    }

    try {
      const [live, ready] = await Promise.all([fetchLiveHealth(), fetchReadyHealth()]);
      if (!mounted) {
        return;
      }

      liveHealth = live;
      readyHealth = ready;
    } catch (error) {
      if (!mounted) {
        return;
      }

      errorMessage =
        error instanceof Error
          ? error.message
          : 'Ops health endpoint belum bisa dibaca.';
    } finally {
      if (mounted) {
        healthLoading = false;
      }
    }
  }

  async function applyFilters() {
    appliedSearchTerm = searchTerm.trim();
    appliedStatusFilter = statusFilter;
    appliedCreatedFrom = createdFrom;
    appliedCreatedTo = createdTo;
    page = 1;
    lastQueueKey = '';
    await loadQueue();
  }

  async function resetFilters() {
    searchTerm = '';
    statusFilter = 'all';
    createdFrom = '';
    createdTo = '';
    appliedSearchTerm = '';
    appliedStatusFilter = 'all';
    appliedCreatedFrom = '';
    appliedCreatedTo = '';
    page = 1;
    lastQueueKey = '';
    await loadQueue();
  }

  function handleStoreScopeChange(event: CustomEvent<{ storeID: string; store: Store | null }>) {
    selectedStoreID = event.detail.storeID;
    selectedStore = event.detail.store;
    setPreferredStoreID(selectedStoreID);
    page = 1;
    lastQueueKey = '';
    void loadQueue();
  }

  function selectCallback(item: CallbackQueueItem) {
    selectedCallbackID = item.id;
    selectedCallback = item;
    attemptsPage = 1;
    lastAttemptsKey = '';
  }

  function queueStatusTone(status: CallbackStatus) {
    switch (status) {
      case 'success':
        return 'text-brand-700 bg-brand-100';
      case 'failed':
        return 'text-rose-700 bg-rose-100';
      case 'retrying':
        return 'text-accent-700 bg-accent-100';
      default:
        return 'text-slate-700 bg-slate-200';
    }
  }

  function summarizeDependencies(report: HealthReport | null) {
    const dependencies = report?.dependencies ?? [];
    return {
      total: dependencies.length,
      ok: dependencies.filter((dependency) => dependency.status === 'ok').length,
      degraded: dependencies.filter((dependency) => dependency.status === 'degraded').length,
      error: dependencies.filter((dependency) => dependency.status === 'error').length,
    };
  }

  function buildQueueChart(summary: CallbackQueueSummary): ChartConfiguration<'doughnut'> {
    return {
      type: 'doughnut',
      data: {
        labels: ['Pending', 'Retrying', 'Success', 'Failed'],
        datasets: [
          {
            data: [
              summary.pending_count,
              summary.retrying_count,
              summary.success_count,
              summary.failed_count,
            ],
            backgroundColor: ['#93a49c', '#efc86d', '#22c977', '#d66b5a'],
            borderWidth: 0,
            hoverOffset: 6,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: {
            position: 'bottom',
            labels: {
              color: chartTextColor,
              boxWidth: 12,
              padding: 18,
            },
          },
        },
      },
    };
  }

  function buildDependencyChart(report: HealthReport | null): ChartConfiguration<'bar'> {
    const dependencies = report?.dependencies ?? [];
    return {
      type: 'bar',
      data: {
        labels: dependencies.map((dependency) => dependency.name),
        datasets: [
          {
            data: dependencies.map((dependency) =>
              dependency.status === 'ok'
                ? 1
                : dependency.status === 'degraded'
                  ? 0.55
                  : 0.2,
            ),
            backgroundColor: dependencies.map((dependency) =>
              dependency.status === 'ok'
                ? '#22c977'
                : dependency.status === 'degraded'
                  ? '#efc86d'
                  : '#d66b5a',
            ),
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
            min: 0,
            max: 1,
            ticks: {
              color: chartTextColor,
              callback(value) {
                if (value === 1) {
                  return 'ok';
                }
                if (value === 0.55) {
                  return 'degraded';
                }
                if (value === 0.2) {
                  return 'error';
                }
                return '';
              },
            },
            grid: {
              color: chartGridColor,
            },
          },
        },
      },
    };
  }

  function exportQueueToCSV() {
    exportRowsToCSV(
      'callback-queue',
      [
        { label: 'Store', value: (item) => item.store_name },
        { label: 'Slug', value: (item) => item.store_slug },
        { label: 'Callback URL', value: (item) => item.callback_url },
        { label: 'Event Type', value: (item) => item.event_type },
        { label: 'Reference Type', value: (item) => item.reference_type },
        { label: 'Reference ID', value: (item) => item.reference_id },
        { label: 'Status', value: (item) => item.status },
        { label: 'Latest Attempt', value: (item) => item.latest_attempt_no },
        { label: 'Latest HTTP', value: (item) => item.latest_http_status ?? '-' },
        { label: 'Created At', value: (item) => formatDateTime(item.created_at) },
        { label: 'Updated At', value: (item) => formatDateTime(item.updated_at) },
      ],
      queueItems,
    );
  }

  function exportQueueToXLSX() {
    return exportRowsToXLSX(
      'callback-queue',
      'Callbacks',
      [
        { label: 'Store', value: (item) => item.store_name },
        { label: 'Slug', value: (item) => item.store_slug },
        { label: 'Callback URL', value: (item) => item.callback_url },
        { label: 'Event Type', value: (item) => item.event_type },
        { label: 'Reference Type', value: (item) => item.reference_type },
        { label: 'Reference ID', value: (item) => item.reference_id },
        { label: 'Status', value: (item) => item.status },
        { label: 'Latest Attempt', value: (item) => item.latest_attempt_no },
        { label: 'Latest HTTP', value: (item) => item.latest_http_status ?? '-' },
        { label: 'Created At', value: (item) => formatDateTime(item.created_at) },
        { label: 'Updated At', value: (item) => formatDateTime(item.updated_at) },
      ],
      queueItems,
    );
  }

  function exportQueueToPDF() {
    return exportRowsToPDF(
      'callback-queue',
      'Callback Queue',
      [
        { label: 'Store', value: (item) => item.store_name },
        { label: 'Slug', value: (item) => item.store_slug },
        { label: 'Event Type', value: (item) => item.event_type },
        { label: 'Reference ID', value: (item) => item.reference_id },
        { label: 'Status', value: (item) => item.status },
        { label: 'Latest Attempt', value: (item) => item.latest_attempt_no },
        { label: 'Latest HTTP', value: (item) => item.latest_http_status ?? '-' },
        { label: 'Updated At', value: (item) => formatDateTime(item.updated_at) },
      ],
      queueItems,
    );
  }

  function exportAttemptsToCSV() {
    exportRowsToCSV(
      `${selectedCallbackID || 'callback'}-attempts`,
      [
        { label: 'Attempt No', value: (attempt) => attempt.attempt_no },
        { label: 'Status', value: (attempt) => attempt.status },
        { label: 'HTTP Status', value: (attempt) => attempt.http_status ?? '-' },
        {
          label: 'Next Retry',
          value: (attempt) => formatDateTime(attempt.next_retry_at),
        },
        { label: 'Created At', value: (attempt) => formatDateTime(attempt.created_at) },
        { label: 'Response', value: (attempt) => attempt.response_body_masked },
      ],
      attempts,
    );
  }

  function exportAttemptsToXLSX() {
    return exportRowsToXLSX(
      `${selectedCallbackID || 'callback'}-attempts`,
      'Attempts',
      [
        { label: 'Attempt No', value: (attempt) => attempt.attempt_no },
        { label: 'Status', value: (attempt) => attempt.status },
        { label: 'HTTP Status', value: (attempt) => attempt.http_status ?? '-' },
        {
          label: 'Next Retry',
          value: (attempt) => formatDateTime(attempt.next_retry_at),
        },
        { label: 'Created At', value: (attempt) => formatDateTime(attempt.created_at) },
        { label: 'Response', value: (attempt) => attempt.response_body_masked },
      ],
      attempts,
    );
  }

  function exportAttemptsToPDF() {
    return exportRowsToPDF(
      `${selectedCallbackID || 'callback'}-attempts`,
      'Callback Attempts',
      [
        { label: 'Attempt No', value: (attempt) => attempt.attempt_no },
        { label: 'Status', value: (attempt) => attempt.status },
        { label: 'HTTP Status', value: (attempt) => attempt.http_status ?? '-' },
        {
          label: 'Next Retry',
          value: (attempt) => formatDateTime(attempt.next_retry_at),
        },
        { label: 'Created At', value: (attempt) => formatDateTime(attempt.created_at) },
      ],
      attempts,
    );
  }
</script>

<svelte:head>
  <title>Ops | onixggr</title>
</svelte:head>

{#if !isPlatformRole}
  <section class="space-y-6">
    <EmptyState
      eyebrow="Restricted Surface"
      title="Ops room hanya untuk dev dan superadmin"
      body="Callback queue, health probes, dan observability ringkas memang disediakan untuk role platform agar owner dan karyawan tidak melihat data lintas tenant."
    />
  </section>
{:else}
  <section class="space-y-6">
    <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
      <div class="grid gap-6 xl:grid-cols-[1.06fr_0.94fr]">
        <div class="space-y-4">
          <span class="status-chip w-fit">Platform ops room</span>
          <div class="space-y-3">
            <p class="section-kicker">Callbacks + health</p>
            <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
              Command surface untuk queue callback dan health origin.
            </h1>
            <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
              Semua read-path di halaman ini bersifat server-side dan tidak di-cache di Redis,
              supaya dev dan superadmin melihat queue callback yang fresh saat incident response.
            </p>
          </div>
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          {#each healthCards as card}
            <MetricCard
              class="h-full"
              eyebrow={card.eyebrow}
              title={card.title}
              value={card.value}
              detail={card.detail}
              tone={card.tone}
            />
          {/each}
        </div>
      </div>
    </section>

    {#if errorMessage !== ''}
      <Notice tone="warning" message={errorMessage} />
    {/if}

    <div class="grid gap-6 xl:grid-cols-[0.78fr_1.22fr]">
      <section class="space-y-6">
        <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Ops filters</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Queue scope
              </h2>
            </div>
            <span class="surface-chip">{formatNumber(queueSummary.total_count)} callback</span>
          </div>

          <div class="mt-5">
            <StoreScopePicker
              bind:selectedStoreID
              bind:selectedStore
              bind:loading={storeScopeLoading}
              bind:totalCount={storeScopeTotalCount}
              allowEmpty
              allowEmptyLabel="All stores"
              compact
              title="Tenant focus"
              description="Pilih satu store untuk isolate callback queue tertentu, atau biarkan kosong untuk melihat seluruh tenant platform."
              placeholder="Cari store, slug, atau callback URL"
              on:change={handleStoreScopeChange}
            />
          </div>

          <div class="mt-5 grid gap-4">
            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Search callback queue</span>
              <input
                bind:value={searchTerm}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                placeholder="Cari store, event_type, callback id, reference id, atau callback URL"
                type="search"
              />
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Status</span>
              <select
                bind:value={statusFilter}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
              >
                <option value="all">Semua status</option>
                <option value="pending">Pending</option>
                <option value="retrying">Retrying</option>
                <option value="success">Success</option>
                <option value="failed">Failed</option>
              </select>
            </label>

            <DateRangeFilter bind:start={createdFrom} bind:end={createdTo} label="Created window" />

            <div class="flex flex-wrap gap-2">
              <Button variant="brand" size="sm" onclick={applyFilters}>
                Apply filters
              </Button>
              <Button variant="outline" size="sm" onclick={resetFilters}>
                Reset
              </Button>
              <Button variant="ghost" size="sm" onclick={() => refreshAll(true)}>
                {refreshing || healthLoading ? 'Refreshing…' : 'Refresh now'}
              </Button>
            </div>
          </div>
        </article>

        <div class="grid gap-6 md:grid-cols-2">
          <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
            <div class="flex items-start justify-between gap-4">
              <div>
                <p class="section-kicker !text-brand-700">Queue mix</p>
                <h2 class="mt-3 font-display text-2xl font-bold tracking-tight text-ink-900">
                  Callback status
                </h2>
              </div>
              <span class="surface-chip">{formatNumber(queueSummary.total_count)} total</span>
            </div>

            <ChartCanvas class="mt-5 min-h-[240px]" config={queueChart} />
          </article>

          <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
            <div class="flex items-start justify-between gap-4">
              <div>
                <p class="section-kicker !text-brand-700">Health posture</p>
                <h2 class="mt-3 font-display text-2xl font-bold tracking-tight text-ink-900">
                  Dependency map
                </h2>
              </div>
              <span class="surface-chip">{formatNumber(dependencyCounts.total)} deps</span>
            </div>

            <ChartCanvas class="mt-5 min-h-[240px]" config={dependencyChart} />
          </article>
        </div>

        <div class="grid gap-6 md:grid-cols-2">
          <GaugeRing
            label="Terminal success rate"
            value={terminalSuccessRate}
            detail="Persentase callback terminal yang berakhir success dibanding success+failed."
            tone="brand"
          />
          <GaugeRing
            label="Attention load"
            value={attentionLoad}
            detail="Proporsi backlog yang masih actionable: pending, retrying, atau failed."
            tone={attentionLoad >= 40 ? 'accent' : 'slate'}
          />
        </div>
      </section>

      <section class="space-y-6">
        <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Queue viewer</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Outbound callback queue
              </h2>
            </div>
            <span class="surface-chip">
              {selectedStore ? selectedStore.slug : 'all stores'}
            </span>
          </div>

          <div class="mt-5 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <MetricCard
              eyebrow="Pending"
              title="Fresh work"
              value={formatNumber(queueSummary.pending_count)}
              detail="Belum pernah dicoba atau baru saja di-enqueue."
              tone="default"
            />
            <MetricCard
              eyebrow="Retrying"
              title="Auto retry"
              value={formatNumber(queueSummary.retrying_count)}
              detail="Sudah pernah gagal dan masih menunggu window retry."
              tone="accent"
            />
            <MetricCard
              eyebrow="Success"
              title="Delivered"
              value={formatNumber(queueSummary.success_count)}
              detail="Callback terminal success."
              tone="brand"
            />
            <MetricCard
              eyebrow="Failed"
              title="Manual follow-up"
              value={formatNumber(queueSummary.failed_count)}
              detail="Sudah habis retry dan butuh investigasi."
              tone="danger"
            />
          </div>

          <div class="mt-5">
            <ExportActions
              count={queueItems.length}
              disabled={queueItems.length === 0}
              onCsv={exportQueueToCSV}
              onXlsx={exportQueueToXLSX}
              onPdf={exportQueueToPDF}
            />
          </div>

          {#if loading}
            <div class="mt-5 space-y-3">
              {#each Array.from({ length: 4 }) as _, index}
                <div class="animate-pulse rounded-[1.6rem] border border-ink-100 bg-canvas-50 px-4 py-4" aria-hidden="true">
                  <div class="h-3 w-20 rounded-full bg-white/80"></div>
                  <div class="mt-3 h-4 w-48 rounded-full bg-white/80"></div>
                  <div class="mt-2 h-3 w-full rounded-full bg-white/75"></div>
                  <div class="mt-2 h-3 w-4/5 rounded-full bg-white/70"></div>
                </div>
              {/each}
            </div>
          {:else if queueItems.length === 0}
            <div class="mt-5">
              <EmptyState
                eyebrow="Callback Queue"
                title="Belum ada callback pada filter ini"
                body="Queue callback kosong atau semua filter terlalu sempit. Ubah status, store scope, atau rentang waktu."
              />
            </div>
          {:else}
            <div class="mt-5 space-y-3 lg:hidden">
              {#each queueItems as item}
                <button
                  class={`w-full rounded-[1.7rem] border p-4 text-left shadow-[0_16px_34px_rgba(7,16,12,0.08)] transition hover:-translate-y-0.5 ${
                    item.id === selectedCallbackID
                      ? 'border-brand-300 bg-brand-100/35'
                      : 'border-ink-100 bg-white/78'
                  }`}
                  onclick={() => selectCallback(item)}
                  type="button"
                >
                  <div class="flex items-start justify-between gap-3">
                    <div>
                      <p class="text-sm font-semibold text-ink-900">{item.store_name}</p>
                      <p class="mt-1 text-xs text-ink-500">{item.store_slug}</p>
                    </div>
                    <span class={`rounded-full px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.22em] ${queueStatusTone(item.status)}`}>
                      {item.status}
                    </span>
                  </div>
                  <p class="mt-4 font-mono text-xs text-ink-900">{item.event_type}</p>
                  <p class="mt-2 text-xs leading-6 text-ink-600">
                    Ref {item.reference_id} · attempt {formatNumber(item.latest_attempt_no)} ·
                    {item.latest_http_status ?? 'no-http'}
                  </p>
                  <p class="mt-2 text-xs leading-6 text-ink-500">
                    Updated {formatRelativeShort(item.updated_at)} · next retry {formatRelativeShort(item.latest_next_retry_at)}
                  </p>
                </button>
              {/each}
            </div>

            <div class="mt-5 hidden overflow-x-auto soft-scroll lg:block">
              <table class="min-w-full border-separate border-spacing-y-3">
                <thead>
                  <tr class="text-left text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-ink-300">
                    <th class="px-3 py-2">Store</th>
                    <th class="px-3 py-2">Event</th>
                    <th class="px-3 py-2">Reference</th>
                    <th class="px-3 py-2">Status</th>
                    <th class="px-3 py-2">Attempts</th>
                    <th class="px-3 py-2">Updated</th>
                  </tr>
                </thead>
                <tbody>
                  {#each queueItems as item}
                    <tr>
                      <td colspan="6" class="p-0">
                        <button
                          class={`w-full rounded-[1.6rem] border px-4 py-4 text-left shadow-[0_16px_34px_rgba(7,16,12,0.08)] transition hover:-translate-y-0.5 ${
                            item.id === selectedCallbackID
                              ? 'border-brand-300 bg-brand-100/32'
                              : 'border-ink-100 bg-white/78'
                          }`}
                          onclick={() => selectCallback(item)}
                          type="button"
                        >
                          <div class="grid gap-3 lg:grid-cols-[1.4fr_1.2fr_1.25fr_0.8fr_0.8fr_1fr] lg:items-center">
                            <div>
                              <p class="text-sm font-semibold text-ink-900">{item.store_name}</p>
                              <p class="mt-1 text-xs text-ink-500">{item.store_slug}</p>
                            </div>
                            <div>
                              <p class="font-mono text-xs text-ink-900">{item.event_type}</p>
                              <p class="mt-1 text-xs text-ink-500">{item.callback_url}</p>
                            </div>
                            <div>
                              <p class="font-mono text-xs text-ink-900">{item.reference_id}</p>
                              <p class="mt-1 text-xs text-ink-500">{item.reference_type}</p>
                            </div>
                            <div>
                              <span class={`rounded-full px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.22em] ${queueStatusTone(item.status)}`}>
                                {item.status}
                              </span>
                            </div>
                            <div class="text-xs leading-6 text-ink-600">
                              <p>#{item.latest_attempt_no}</p>
                              <p>{item.latest_http_status ?? 'no-http'}</p>
                            </div>
                            <div class="text-xs leading-6 text-ink-500">
                              <p>{formatRelativeShort(item.updated_at)}</p>
                              <p>{formatDateTime(item.updated_at)}</p>
                            </div>
                          </div>
                        </button>
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>

            <div class="mt-5">
              <PaginationControls
                bind:page
                bind:pageSize
                totalItems={queueSummary.total_count}
                pageSizeOptions={[6, 12, 24, 48]}
              />
            </div>
          {/if}
        </article>

        <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Attempts panel</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Delivery attempts
              </h2>
            </div>
            {#if selectedCallback}
              <span class="surface-chip">{selectedCallback.event_type}</span>
            {/if}
          </div>

          {#if selectedCallback}
            <div class="mt-5 rounded-[1.7rem] border border-ink-100 bg-canvas-50 px-4 py-4">
              <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <p class="text-sm font-semibold text-ink-900">{selectedCallback.store_name}</p>
                  <p class="mt-1 text-xs text-ink-500">{selectedCallback.callback_url}</p>
                </div>
                <span class={`rounded-full px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.22em] ${queueStatusTone(selectedCallback.status)}`}>
                  {selectedCallback.status}
                </span>
              </div>

              <div class="mt-4 grid gap-4 md:grid-cols-2">
                <div class="rounded-[1.4rem] bg-white px-4 py-4 shadow-[0_12px_24px_rgba(7,16,12,0.06)]">
                  <p class="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-ink-300">Reference</p>
                  <p class="mt-3 font-mono text-xs text-ink-900">{selectedCallback.reference_id}</p>
                  <p class="mt-2 text-xs text-ink-500">{selectedCallback.reference_type}</p>
                </div>
                <div class="rounded-[1.4rem] bg-white px-4 py-4 shadow-[0_12px_24px_rgba(7,16,12,0.06)]">
                  <p class="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-ink-300">Latest retry window</p>
                  <p class="mt-3 text-sm font-semibold text-ink-900">
                    {formatDateTime(selectedCallback.latest_next_retry_at)}
                  </p>
                  <p class="mt-2 text-xs text-ink-500">
                    Updated {formatRelativeShort(selectedCallback.updated_at)}
                  </p>
                </div>
              </div>
            </div>

            <div class="mt-5">
              <ExportActions
                count={attempts.length}
                disabled={attempts.length === 0}
                onCsv={exportAttemptsToCSV}
                onXlsx={exportAttemptsToXLSX}
                onPdf={exportAttemptsToPDF}
              />
            </div>

            {#if attemptsError !== ''}
              <div class="mt-5">
                <Notice tone="warning" message={attemptsError} />
              </div>
            {:else if attemptsLoading}
              <div class="mt-5 space-y-3">
                {#each Array.from({ length: 3 }) as _, index}
                  <div class="animate-pulse rounded-[1.4rem] border border-ink-100 bg-canvas-50 px-4 py-4" aria-hidden="true">
                    <div class="h-3 w-20 rounded-full bg-white/80"></div>
                    <div class="mt-3 h-3 w-40 rounded-full bg-white/75"></div>
                    <div class="mt-2 h-3 w-full rounded-full bg-white/70"></div>
                  </div>
                {/each}
              </div>
            {:else if attempts.length === 0}
              <div class="mt-5">
                <EmptyState
                  eyebrow="Attempt history"
                  title="Callback ini belum punya attempt log"
                  body="Biasanya ini berarti callback masih fresh pending dan belum pernah diproses worker."
                />
              </div>
            {:else}
              <div class="mt-5 space-y-3">
                {#each attempts as attempt}
                  <article class="rounded-[1.6rem] border border-ink-100 bg-white/78 px-4 py-4 shadow-[0_16px_34px_rgba(7,16,12,0.08)]">
                    <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                      <div>
                        <div class="flex flex-wrap items-center gap-2">
                          <span class="surface-chip">Attempt #{attempt.attempt_no}</span>
                          <span class={`rounded-full px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.22em] ${attempt.status === 'success' ? 'text-brand-700 bg-brand-100' : 'text-rose-700 bg-rose-100'}`}>
                            {attempt.status}
                          </span>
                        </div>
                        <p class="mt-3 text-sm font-semibold text-ink-900">
                          HTTP {attempt.http_status ?? '-'} · {formatDateTime(attempt.created_at)}
                        </p>
                        <p class="mt-1 text-xs text-ink-500">
                          Next retry {formatDateTime(attempt.next_retry_at)}
                        </p>
                      </div>
                      <span class="surface-chip">{formatRelativeShort(attempt.created_at)}</span>
                    </div>

                    <pre class="code-block mt-4">{attempt.response_body_masked || 'response body masked not available'}</pre>
                  </article>
                {/each}
              </div>

              <div class="mt-5">
                <PaginationControls
                  bind:page={attemptsPage}
                  bind:pageSize={attemptsPageSize}
                  totalItems={attemptsTotalCount}
                  pageSizeOptions={[4, 6, 12]}
                />
              </div>
            {/if}
          {:else}
            <div class="mt-5">
              <EmptyState
                eyebrow="Attempts panel"
                title="Pilih satu callback dari queue"
                body="Detail attempt, masked response, dan retry window akan tampil di sini setelah Anda memilih satu row callback."
              />
            </div>
          {/if}
        </article>
      </section>
    </div>
  </section>
{/if}
