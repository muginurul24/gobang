<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import DateRangeFilter from '$lib/components/app/date-range-filter.svelte';
  import EmptyState from '$lib/components/app/empty-state.svelte';
  import ExportActions from '$lib/components/app/export-actions.svelte';
  import Button from '$lib/components/ui/button/button.svelte';
  import { authSession, initializeAuthSession } from '$lib/auth/client';
  import { fetchAuditLogs, type AuditLogEntry } from '$lib/audit/client';
  import { exportRowsToCSV, exportRowsToPDF, exportRowsToXLSX } from '$lib/exporters';
  import MetricCard from '$lib/components/app/metric-card.svelte';
  import PaginationControls from '$lib/components/app/pagination-controls.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import PageSkeleton from '$lib/components/app/page-skeleton.svelte';
  import StoreScopePicker from '$lib/components/app/store-scope-picker.svelte';
  import type { Store } from '$lib/stores/client';
  import { formatDateTime, formatNumber } from '$lib/formatters';

  let loading = true;
  let refreshing = false;
  let errorMessage = '';
  let logs: AuditLogEntry[] = [];
  let storeScopeLoading = true;
  let storeScopeTotalCount = 0;
  let selectedStoreID = '';
  let selectedStore: Store | null = null;
  let actionQuery = '';
  let selectedActorRole = '';
  let selectedTargetType = '';
  let auditFrom = '';
  let auditTo = '';
  let auditPage = 1;
  let auditPageSize = 12;
  let totalCount = 0;
  let lastAuditQueryKey = '';

  $: uniqueActions = new Set(logs.map((entry) => entry.action)).size;
  $: uniqueStores = new Set(logs.map((entry) => entry.store_id).filter(Boolean)).size;

  onMount(async () => {
    await initializeAuthSession();

    if (!$authSession) {
      await goto('/login');
      return;
    }

    await loadAudit();
  });

  $: {
    const nextKey = `${auditPage}:${auditPageSize}`;
    if (!loading && nextKey !== lastAuditQueryKey) {
      lastAuditQueryKey = nextKey;
      void refreshAudit();
    }
  }

  async function loadAudit() {
    loading = true;
    errorMessage = '';

    const logsResponse = await fetchAuditLogs({
      storeID: selectedStoreID || undefined,
      limit: auditPageSize,
      offset: (auditPage - 1) * auditPageSize,
      action: actionQuery || undefined,
      actorRole: selectedActorRole || undefined,
      targetType: selectedTargetType || undefined,
      createdFrom: auditFrom || undefined,
      createdTo: auditTo || undefined
    });
    if (!(await ensureAuthorized(logsResponse.message))) {
      return;
    }

    if (!logsResponse.status || logsResponse.message !== 'SUCCESS') {
      errorMessage = toMessage(logsResponse.message);
      logs = [];
      loading = false;
      return;
    }

    logs = logsResponse.data.items ?? [];
    totalCount = logsResponse.data.total_count ?? 0;
    lastAuditQueryKey = `${auditPage}:${auditPageSize}`;
    loading = false;
  }

  async function handleStoreScopeChange(event: CustomEvent<{ storeID: string; store: Store | null }>) {
    selectedStoreID = event.detail.storeID;
    selectedStore = event.detail.store;
    auditPage = 1;
    await refreshAudit();
  }

  async function refreshAudit() {
    refreshing = true;
    await loadAudit();
    refreshing = false;
  }

  async function ensureAuthorized(message: string) {
    if (message !== 'UNAUTHORIZED') {
      return true;
    }

    await goto('/login');
    return false;
  }

  function formatPayload(entry: AuditLogEntry) {
    if (!entry.payload_masked) {
      return '{}';
    }

    return JSON.stringify(entry.payload_masked, null, 2);
  }

  function toMessage(message: string) {
    switch (message) {
      case 'FORBIDDEN':
        return 'Audit viewer hanya tersedia untuk owner, superadmin, dan dev.';
      default:
        return 'Gagal memuat audit log.';
    }
  }

  function exportAuditToCSV() {
    exportRowsToCSV(
      `${selectedStoreID || 'scope'}-audit`,
      [
        { label: 'Created At', value: (entry) => formatDateTime(entry.created_at) },
        { label: 'Action', value: (entry) => entry.action },
        { label: 'Actor Role', value: (entry) => entry.actor_role },
        { label: 'Store ID', value: (entry) => entry.store_id ?? '-' },
        { label: 'Target Type', value: (entry) => entry.target_type },
        { label: 'Target ID', value: (entry) => entry.target_id ?? '-' },
        { label: 'IP Address', value: (entry) => entry.ip_address ?? '-' },
        { label: 'Payload', value: (entry) => formatPayload(entry) }
      ],
      logs,
    );
  }

  function exportAuditToXLSX() {
    exportRowsToXLSX(
      `${selectedStoreID || 'scope'}-audit`,
      'Audit',
      [
        { label: 'Created At', value: (entry) => formatDateTime(entry.created_at) },
        { label: 'Action', value: (entry) => entry.action },
        { label: 'Actor Role', value: (entry) => entry.actor_role },
        { label: 'Store ID', value: (entry) => entry.store_id ?? '-' },
        { label: 'Target Type', value: (entry) => entry.target_type },
        { label: 'Target ID', value: (entry) => entry.target_id ?? '-' },
        { label: 'IP Address', value: (entry) => entry.ip_address ?? '-' },
        { label: 'Payload', value: (entry) => formatPayload(entry) }
      ],
      logs,
    );
  }

  function exportAuditToPDF() {
    exportRowsToPDF(
      `${selectedStoreID || 'scope'}-audit`,
      'Audit Trail',
      [
        { label: 'Created', value: (entry) => formatDateTime(entry.created_at) },
        { label: 'Action', value: (entry) => entry.action },
        { label: 'Role', value: (entry) => entry.actor_role },
        { label: 'Store', value: (entry) => entry.store_id ?? '-' },
        { label: 'Target', value: (entry) => `${entry.target_type}:${entry.target_id ?? '-'}` }
      ],
      logs,
    );
  }
</script>

<svelte:head>
  <title>Audit | onixggr</title>
</svelte:head>

{#if loading}
  <PageSkeleton blocks={3} />
{:else}
  <div class="space-y-6">
    <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
      <div class="flex flex-col gap-5 md:flex-row md:items-start md:justify-between">
        <div class="space-y-2">
          <p class="section-kicker">
            Audit Viewer
          </p>
          <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
            Audit trail untuk boundary owner, staff, dan platform role.
          </h1>
          <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
            Owner hanya melihat domainnya, superadmin/dev melihat global, dan karyawan diblokir dari
            endpoint audit. Audit log dipertahankan maksimal 90 hari lewat retention job scheduler.
          </p>
        </div>

        <div class="rounded-[1.8rem] border border-white/12 bg-white/7 px-4 py-4 text-sm text-white/72 backdrop-blur">
          <p class="font-semibold text-white">Filter</p>
          <p class="mt-2">Role: {$authSession?.user.role ?? '-'}</p>
          <p>Total row: {formatNumber(totalCount)}</p>
        </div>
      </div>
    </section>

    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <MetricCard
        eyebrow="Rows"
        title="Filtered logs"
        value={formatNumber(totalCount)}
        detail="Total row audit terfilter yang tersedia dari backend."
        tone="brand"
      />
      <MetricCard
        eyebrow="Action"
        title="Unique actions"
        value={formatNumber(uniqueActions)}
        detail="Variasi action yang muncul dalam hasil saat ini."
      />
      <MetricCard
        eyebrow="Store"
        title="Store touched"
        value={formatNumber(uniqueStores)}
        detail="Jumlah store unik yang muncul pada result set saat ini."
      />
      <MetricCard
        eyebrow="Scope"
        title="Viewer role"
        value={$authSession?.user.role ?? '-'}
        detail="Owner scoped, superadmin/dev global, karyawan diblokir."
        tone="accent"
      />
    </div>

    {#if errorMessage}
      <Notice tone="error" title="Audit belum bisa dimuat" message={errorMessage} />
    {/if}

    <section class="glass-panel rounded-4xl p-6">
      <StoreScopePicker
        bind:selectedStoreID
        bind:selectedStore
        bind:loading={storeScopeLoading}
        bind:totalCount={storeScopeTotalCount}
        title="Store filter untuk audit"
        description="Audit viewer tetap global secara default. Aktifkan filter store hanya saat Anda memang ingin menyempitkan result set."
        placeholder="Cari store untuk menyaring audit"
        allowEmpty={true}
        allowEmptyLabel="Semua store dalam scope"
        on:change={handleStoreScopeChange}
      />

      <div class="mt-5 grid gap-4 xl:grid-cols-[180px_180px_220px_auto]">

        <label class="space-y-2">
          <span class="text-sm font-medium text-ink-700">Legacy window</span>
          <select
            bind:value={auditPageSize}
            class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
          >
            <option value={25}>25</option>
            <option value={50}>50</option>
            <option value={100}>100</option>
            <option value={200}>200</option>
          </select>
        </label>

        <label class="space-y-2">
          <span class="text-sm font-medium text-ink-700">Actor role</span>
          <select
            bind:value={selectedActorRole}
            class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
          >
            <option value="">Semua role actor</option>
            <option value="owner">owner</option>
            <option value="karyawan">karyawan</option>
            <option value="dev">dev</option>
            <option value="superadmin">superadmin</option>
            <option value="store_api">store_api</option>
            <option value="provider_webhook">provider_webhook</option>
            <option value="system">system</option>
            <option value="guest">guest</option>
          </select>
        </label>

        <label class="space-y-2">
          <span class="text-sm font-medium text-ink-700">Target type</span>
          <select
            bind:value={selectedTargetType}
            class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
          >
            <option value="">Semua target</option>
            <option value="auth">auth</option>
            <option value="user">user</option>
            <option value="store">store</option>
            <option value="store_member">store_member</option>
            <option value="game_transaction">game_transaction</option>
            <option value="qris_transaction">qris_transaction</option>
            <option value="store_withdrawal">store_withdrawal</option>
          </select>
        </label>

        <div class="flex items-end">
          <Button variant="brand" size="lg" onclick={refreshAudit} disabled={refreshing}>
            Refresh Audit
          </Button>
        </div>
      </div>

      <div class="mt-4 grid gap-4 md:grid-cols-[1fr_auto]">
        <label class="space-y-2">
          <span class="text-sm font-medium text-ink-700">Action contains</span>
          <input
            bind:value={actionQuery}
            class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
            placeholder="contoh: withdraw, auth.login, store.token"
            type="text"
          />
        </label>

        <div class="flex items-end">
          <Button
            variant="outline"
            size="lg"
            onclick={() => {
              actionQuery = '';
              selectedActorRole = '';
              selectedTargetType = '';
              selectedStoreID = '';
              auditFrom = '';
              auditTo = '';
              auditPage = 1;
              lastAuditQueryKey = '';
              void refreshAudit();
            }}
          >
            Reset Filter
          </Button>
        </div>
      </div>

      <div class="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(18rem,24rem)]">
        <DateRangeFilter bind:start={auditFrom} bind:end={auditTo} label="Created at" />
        <ExportActions
          count={logs.length}
          disabled={logs.length === 0}
          onCsv={exportAuditToCSV}
          onXlsx={exportAuditToXLSX}
          onPdf={exportAuditToPDF}
        />
      </div>

      <p class="mt-4 text-xs leading-5 text-ink-500">
        Pagination, action filter, role filter, store scope, dan date range sekarang dieksekusi di backend agar tetap ringan saat row audit besar.
      </p>

      <div class="mt-4 flex flex-wrap gap-3">
        <Button
          variant="brand"
          size="sm"
          onclick={async () => {
            auditPage = 1;
            lastAuditQueryKey = '';
            await loadAudit();
          }}
          disabled={refreshing}
        >
          Apply filters
        </Button>
      </div>
    </section>

    <section class="space-y-4">
      {#if totalCount === 0}
        <EmptyState
          eyebrow="Audit Window"
          title="Tidak ada audit log"
          body="Tidak ada audit log dalam scope dan filter backend saat ini."
        />
      {:else}
        {#each logs as entry}
          <article class="glass-panel rounded-4xl p-6">
            <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
              <div class="space-y-1">
                <p class="text-xs font-semibold uppercase tracking-[0.24em] text-accent-700">
                  {entry.action}
                </p>
                <h2 class="font-display text-2xl font-bold text-ink-900">{entry.target_type}</h2>
                <p class="text-sm text-ink-700">
                  Actor role: <span class="font-semibold text-ink-900">{entry.actor_role}</span>
                </p>
                <p class="text-xs text-ink-500">Actor ID: {entry.actor_user_id ?? '-'}</p>
              </div>

              <div class="rounded-[1.6rem] bg-canvas-100 px-4 py-3 text-sm text-ink-700">
                <p class="font-semibold text-ink-900">Meta</p>
                <p>Store: {entry.store_id ?? '-'}</p>
                <p>Target: {entry.target_id ?? '-'}</p>
                <p>IP: {entry.ip_address ?? '-'}</p>
                <p>{formatDateTime(entry.created_at)}</p>
              </div>
            </div>

            <div class="mt-5 rounded-3xl border border-ink-100 bg-white p-4">
              <p class="text-xs font-semibold uppercase tracking-[0.24em] text-ink-300">
                Payload Masked
              </p>
              <pre class="mt-3 overflow-x-auto text-xs leading-6 text-ink-700">{formatPayload(entry)}</pre>
            </div>
          </article>
        {/each}

        <PaginationControls bind:page={auditPage} bind:pageSize={auditPageSize} totalItems={totalCount} />
      {/if}
    </section>
  </div>
{/if}
