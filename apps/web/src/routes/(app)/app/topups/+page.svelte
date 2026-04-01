<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import type { ChartConfiguration } from 'chart.js';
  import QRCode from 'qrcode';

  import { authSession, initializeAuthSession } from '$lib/auth/client';
  import { chartTextColor as resolveChartTextColor } from '$lib/chart-theme';
  import ChartCanvas from '$lib/components/app/chart-canvas.svelte';
  import DateRangeFilter from '$lib/components/app/date-range-filter.svelte';
  import EmptyState from '$lib/components/app/empty-state.svelte';
  import ExportActions from '$lib/components/app/export-actions.svelte';
  import MetricCard from '$lib/components/app/metric-card.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import PaginationControls from '$lib/components/app/pagination-controls.svelte';
  import PageSkeleton from '$lib/components/app/page-skeleton.svelte';
  import StoreScopePicker from '$lib/components/app/store-scope-picker.svelte';
  import Button from '$lib/components/ui/button/button.svelte';
  import { exportRowsToCSV, exportRowsToPDF, exportRowsToXLSX } from '$lib/exporters';
  import { formatCurrency, formatDateTime, formatNumber } from '$lib/formatters';
  import {
    createStoreTopup,
    fetchStoreTopups,
    type StoreTopup
  } from '$lib/payments-qris/client';
  import { isStoreLowBalance, type Store } from '$lib/stores/client';
  import {
    hydratePreferredStoreID,
    preferredStoreID,
    setPreferredStoreID
  } from '$lib/stores/preferences';
  import { resolvedTheme } from '$lib/theme';

  let loading = true;
  let busy = false;
  let errorMessage = '';
  let successMessage = '';
  let storeScopeLoading = true;
  let storeScopeTotalCount = 0;
  let selectedStoreID = '';
  let selectedStore: Store | null = null;
  let topups: StoreTopup[] = [];
  let transactionType: StoreTopup['type'] = 'store_topup';
  let selectedTopupID = '';
  let statusFilter: StoreTopup['status'] | 'all' = 'all';
  let amountInput = '';
  let topupSearchTerm = '';
  let topupCreatedFrom = '';
  let topupCreatedTo = '';
  let topupPage = 1;
  let topupPageSize = 6;
  let topupTotalCount = 0;
  let qrCodeDataURL = '';
  let qrRequestID = 0;
  let selectedTopup: StoreTopup | null = null;
  let lastTopupPaginationKey = '';

  let pendingCount = 0;
  let successCount = 0;
  let expiredCount = 0;
  let failedCount = 0;
  let totalGross = 0;
  let pendingGross = 0;
  $: chartTextColor = resolveChartTextColor($resolvedTheme);
  $: topupMixChart = buildTopupMixChart([
    pendingCount,
    successCount,
    expiredCount,
    failedCount
  ]);
  $: amountError =
    amountInput.trim() !== '' && !/^[1-9][0-9]*$/.test(amountInput.trim())
      ? 'Nominal topup harus angka bulat lebih dari nol.'
      : '';

  onMount(() => {
    let active = true;
    hydratePreferredStoreID();
    const unsubscribe = preferredStoreID.subscribe(async (storeID) => {
      if (!active || loading || storeScopeLoading) {
        return;
      }

      if (storeID !== '' && storeID !== selectedStoreID) {
        selectedStoreID = storeID;
        errorMessage = '';
        successMessage = '';
        await loadTopups();
      }
    });

    void (async () => {
      await initializeAuthSession();

      if (!$authSession) {
        await goto('/login');
        return;
      }

      loading = false;
    })();

    return () => {
      active = false;
      unsubscribe();
    };
  });

  $: selectedTopup = topups.find((topup) => topup.id === selectedTopupID) ?? null;

  $: {
    void updateQRCode(selectedTopup?.qr_code_value ?? null);
  }

  $: if (!loading && selectedStoreID !== '') {
    const nextKey = `${selectedStoreID}:${topupPage}:${topupPageSize}`;
    if (nextKey !== lastTopupPaginationKey) {
      lastTopupPaginationKey = nextKey;
      void loadTopups(selectedTopupID);
    }
  }

  async function loadTopups(preferredTopupID = '') {
    if (selectedStoreID === '') {
      topups = [];
      topupTotalCount = 0;
      pendingCount = 0;
      successCount = 0;
      expiredCount = 0;
      failedCount = 0;
      totalGross = 0;
      pendingGross = 0;
      selectedTopupID = '';
      lastTopupPaginationKey = '';
      return;
    }

    const response = await fetchStoreTopups(selectedStoreID, {
      type: transactionType,
      status: statusFilter,
      query: topupSearchTerm,
      limit: topupPageSize,
      offset: (topupPage - 1) * topupPageSize,
      createdFrom: topupCreatedFrom,
      createdTo: topupCreatedTo
    });
    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      topups = [];
      topupTotalCount = 0;
      pendingCount = 0;
      successCount = 0;
      expiredCount = 0;
      failedCount = 0;
      totalGross = 0;
      pendingGross = 0;
      selectedTopupID = '';
      lastTopupPaginationKey = `${selectedStoreID}:${topupPage}:${topupPageSize}`;
      return;
    }

    topups = response.data.items ?? [];
    topupTotalCount = response.data.summary?.total_count ?? 0;
    pendingCount = response.data.summary?.pending_count ?? 0;
    successCount = response.data.summary?.success_count ?? 0;
    expiredCount = response.data.summary?.expired_count ?? 0;
    failedCount = response.data.summary?.failed_count ?? 0;
    totalGross = Number(response.data.summary?.total_gross ?? 0);
    pendingGross = Number(response.data.summary?.pending_gross ?? 0);
    lastTopupPaginationKey = `${selectedStoreID}:${topupPage}:${topupPageSize}`;

    if (preferredTopupID !== '' && topups.some((topup) => topup.id === preferredTopupID)) {
      selectedTopupID = preferredTopupID;
      return;
    }

    selectedTopupID = pickPreferredTopup(topups)?.id ?? '';
  }

  async function handleStoreScopeChange(event: CustomEvent<{ storeID: string; store: Store | null }>) {
    selectedStoreID = event.detail.storeID;
    selectedStore = event.detail.store;
    setPreferredStoreID(selectedStoreID);
    topupPage = 1;
    lastTopupPaginationKey = '';
    errorMessage = '';
    successMessage = '';
    await loadTopups();
  }

  async function submitCreateTopup() {
    if (selectedStoreID === '') {
      errorMessage = 'Pilih toko yang akan di-topup terlebih dahulu.';
      return;
    }

    if (amountInput.trim() === '') {
      errorMessage = 'Masukkan nominal topup dalam rupiah.';
      return;
    }

    if (amountError !== '') {
      errorMessage = amountError;
      return;
    }

    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await createStoreTopup(selectedStoreID, Number(amountInput.trim()));
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || !['SUCCESS', 'PENDING_PROVIDER'].includes(response.message)) {
      errorMessage = toMessage(response.message);
      return;
    }

    amountInput = '';
    successMessage =
      response.message === 'SUCCESS'
        ? 'Topup QRIS dibuat. QR code aktif bisa langsung dipindai.'
        : 'Transaksi pending dibuat, tetapi respons generate provider masih ambigu.';
    transactionType = 'store_topup';
    statusFilter = 'all';
    topupPage = 1;
    lastTopupPaginationKey = '';
    await loadTopups(response.data.id);
  }

  async function applyFilters() {
    topupPage = 1;
    lastTopupPaginationKey = '';
    await loadTopups(selectedTopupID);
  }

  async function resetFilters() {
    transactionType = 'store_topup';
    statusFilter = 'all';
    topupSearchTerm = '';
    topupCreatedFrom = '';
    topupCreatedTo = '';
    topupPage = 1;
    lastTopupPaginationKey = '';
    await loadTopups(selectedTopupID);
  }

  async function updateQRCode(rawValue: string | null) {
    const requestID = ++qrRequestID;
    qrCodeDataURL = '';

    if (!rawValue) {
      return;
    }

    try {
      const dataURL = await QRCode.toDataURL(rawValue, {
        errorCorrectionLevel: 'M',
        margin: 1,
        width: 360
      });

      if (requestID === qrRequestID) {
        qrCodeDataURL = dataURL;
      }
    } catch {
      if (requestID === qrRequestID) {
        qrCodeDataURL = '';
      }
    }
  }

  function pickPreferredTopup(entries: StoreTopup[]) {
    return (
      entries.find((entry) => entry.status === 'pending' && entry.qr_code_value) ??
      entries.find((entry) => entry.status === 'pending') ??
      entries[0] ??
      null
    );
  }

  function canManageTopups() {
    return ['owner', 'dev', 'superadmin'].includes($authSession?.user.role ?? '');
  }

  function currentStore() {
    return selectedStore;
  }

  function hasLowBalanceStore() {
    const store = currentStore();
    return store ? isStoreLowBalance(store) : false;
  }

  function statusLabel(status: StoreTopup['status']) {
    switch (status) {
      case 'success':
        return 'Success';
      case 'expired':
        return 'Expired';
      case 'failed':
        return 'Failed';
      default:
        return 'Pending';
    }
  }

  function statusClass(status: StoreTopup['status']) {
    switch (status) {
      case 'success':
        return 'border-emerald-200 bg-emerald-50 text-emerald-700';
      case 'expired':
        return 'border-ink-200 bg-canvas-100 text-ink-700';
      case 'failed':
        return 'border-rose-200 bg-rose-50 text-rose-700';
      default:
        return 'border-amber-200 bg-amber-50 text-amber-700';
    }
  }

  function providerNote(topup: StoreTopup) {
    switch (topup.provider_state) {
      case 'generated':
        return 'QR berhasil dibuat dan masih menunggu pembayaran.';
      case 'pending_provider_response':
        return 'Generate provider ambigu. Tunggu webhook atau buat ulang topup baru.';
      case 'generate_failed':
        return 'Generate QR gagal sebelum provider mengembalikan transaksi aktif.';
      default:
        return 'Transaksi baru dibuat dan menunggu sinkronisasi provider.';
    }
  }

  async function ensureAuthorized(message: string) {
    if (message !== 'UNAUTHORIZED') {
      return true;
    }

    await goto('/login');
    return false;
  }

  function toMessage(message: string) {
    switch (message) {
      case 'FORBIDDEN':
        return 'Role Anda tidak bisa membuat atau melihat topup QRIS toko.';
      case 'NOT_FOUND':
        return 'Store yang diminta tidak ditemukan.';
      case 'STORE_INACTIVE':
        return 'Topup QRIS hanya bisa dibuat untuk store aktif.';
      case 'INVALID_AMOUNT':
        return 'Nominal topup harus angka bulat lebih dari nol.';
      case 'UPSTREAM_NOT_CONFIGURED':
        return 'Provider QRIS belum dikonfigurasi di environment ini.';
      case 'PENDING_PROVIDER':
        return 'Provider belum memberi jawaban final. Transaksi tetap tersimpan sebagai pending.';
      default:
        return 'Terjadi kesalahan. Coba ulangi.';
    }
  }

  function buildTopupMixChart(values: number[]): ChartConfiguration<'doughnut'> {
    return {
      type: 'doughnut',
      data: {
        labels: ['Pending', 'Success', 'Expired', 'Failed'],
        datasets: [
          {
            data: values,
            backgroundColor: ['#efc86d', '#22c977', '#92826c', '#d66b5a'],
            borderWidth: 0,
            hoverOffset: 6
          }
        ]
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
              padding: 18
            }
          }
        }
      }
    };
  }

  function exportTopupsToCSV() {
    exportRowsToCSV(
      `${selectedStoreID || 'store'}-qris-topups`,
      [
        { label: 'Status', value: (topup) => statusLabel(topup.status) },
        { label: 'Type', value: (topup) => topup.type },
        { label: 'Custom Ref', value: (topup) => topup.custom_ref },
        { label: 'External Username', value: (topup) => topup.external_username },
        { label: 'Amount Gross', value: (topup) => topup.amount_gross },
        { label: 'Store Credit Amount', value: (topup) => topup.store_credit_amount },
        { label: 'Platform Fee', value: (topup) => topup.platform_fee_amount },
        { label: 'Provider Trx ID', value: (topup) => topup.provider_trx_id ?? '-' },
        { label: 'Provider State', value: (topup) => topup.provider_state ?? '-' },
        { label: 'Created At', value: (topup) => formatDateTime(topup.created_at) },
        { label: 'Expires At', value: (topup) => formatDateTime(topup.expires_at) }
      ],
      topups,
    );
  }

  function exportTopupsToXLSX() {
    exportRowsToXLSX(
      `${selectedStoreID || 'store'}-qris-topups`,
      'Topups',
      [
        { label: 'Status', value: (topup) => statusLabel(topup.status) },
        { label: 'Type', value: (topup) => topup.type },
        { label: 'Custom Ref', value: (topup) => topup.custom_ref },
        { label: 'External Username', value: (topup) => topup.external_username },
        { label: 'Amount Gross', value: (topup) => topup.amount_gross },
        { label: 'Store Credit Amount', value: (topup) => topup.store_credit_amount },
        { label: 'Platform Fee', value: (topup) => topup.platform_fee_amount },
        { label: 'Provider Trx ID', value: (topup) => topup.provider_trx_id ?? '-' },
        { label: 'Provider State', value: (topup) => topup.provider_state ?? '-' },
        { label: 'Created At', value: (topup) => formatDateTime(topup.created_at) },
        { label: 'Expires At', value: (topup) => formatDateTime(topup.expires_at) }
      ],
      topups,
    );
  }

  function exportTopupsToPDF() {
    exportRowsToPDF(
      `${selectedStoreID || 'store'}-qris-topups`,
      'QRIS Topup History',
      [
        { label: 'Status', value: (topup) => statusLabel(topup.status) },
        { label: 'Ref', value: (topup) => topup.custom_ref },
        { label: 'Username', value: (topup) => topup.external_username },
        { label: 'Gross', value: (topup) => formatCurrency(topup.amount_gross) },
        { label: 'Credit', value: (topup) => formatCurrency(topup.store_credit_amount) },
        { label: 'Provider', value: (topup) => topup.provider_trx_id ?? '-' },
        { label: 'Created', value: (topup) => formatDateTime(topup.created_at) }
      ],
      topups,
    );
  }
</script>

<svelte:head>
  <title>QRIS Desk | onixggr</title>
</svelte:head>

{#if loading}
  <PageSkeleton blocks={5} />
{:else}
  <div class="space-y-6">
    <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
      <div class="flex flex-col gap-5 md:flex-row md:items-start md:justify-between">
        <div class="space-y-2">
          <p class="section-kicker">
            QRIS desk
          </p>
          <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
            Command surface untuk store topup dan member payment QRIS.
          </h1>
          <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
            Dashboard ini tetap membuat `store_topup` dari browser, sambil juga membaca histori
            `member_payment` dari website owner yang memukul store API. Kedua jalur memakai domain
            transaksi terpisah, tetapi berbagi engine QRIS, webhook, dan reconcile yang sama.
          </p>
        </div>

        <div class="rounded-[1.8rem] border border-white/12 bg-white/7 px-4 py-4 text-sm text-white/72 backdrop-blur">
          <p class="font-semibold text-white">Scope</p>
          <p class="mt-2">Role: {$authSession?.user.role ?? '-'}</p>
          <p>Toko tersedia: {formatNumber(storeScopeTotalCount)}</p>
        </div>
      </div>
    </section>

    {#if errorMessage}
      <Notice tone="error" title="Generate topup belum berhasil" message={errorMessage} />
    {/if}

    {#if successMessage}
      <Notice tone="success" title="Topup tersimpan" message={successMessage} />
    {/if}

    {#if !canManageTopups()}
      <EmptyState
        eyebrow="Role Scope"
        title="Role ini tidak bisa mengelola topup QRIS"
        body="Topup dashboard hanya tersedia untuk owner, dev, dan superadmin agar flow uang tidak dipicu dari role yang salah."
      />
    {:else if storeScopeLoading}
      <PageSkeleton blocks={2} />
    {:else if storeScopeTotalCount === 0}
      <EmptyState
        eyebrow="Store Topup"
        title="Belum ada toko untuk di-topup"
        body="Tambahkan toko lebih dulu atau pastikan store yang relevan memang ada di scope sesi dashboard ini."
        actionHref="/app/stores"
        actionLabel="Buka Stores"
      />
    {:else}
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          eyebrow="Queue"
          title="Pending topups"
          value={formatNumber(pendingCount)}
          detail="Generate QR yang belum final atau masih menunggu pembayaran."
          tone="accent"
        />
        <MetricCard
          eyebrow="Success"
          title="Settled QRIS"
          value={formatNumber(successCount)}
          detail="Histori QRIS sukses untuk tipe transaksi yang sedang difilter."
          tone="brand"
        />
        <MetricCard
          eyebrow="Gross"
          title="Total requested"
          value={formatCurrency(totalGross)}
          detail="Akumulasi nominal gross di histori topup store aktif."
        />
        <MetricCard
          eyebrow="Pending Gross"
          title="Open QR value"
          value={formatCurrency(pendingGross)}
          detail="Nominal gross yang masih terbuka di QR pending."
          tone="accent"
        />
      </div>

      <div class="grid gap-6 xl:grid-cols-[0.95fr_1.05fr]">
        <section class="glass-panel rounded-4xl p-6">
          <h2 class="font-display text-2xl font-bold text-ink-900">Buat topup baru</h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            Provider generate akan memakai `username` owner store, `custom_ref` internal topup,
            dan `uuid` global dari konfigurasi QRIS.
          </p>

          <div class="mt-5 space-y-4">
            <StoreScopePicker
              bind:selectedStoreID
              bind:selectedStore
              bind:loading={storeScopeLoading}
              bind:totalCount={storeScopeTotalCount}
              compact
              title="Store scope untuk topup QRIS"
              description="Selector ini memakai store directory backend yang sudah dipaginasi, jadi tetap cepat saat roster store membesar."
              placeholder="Cari store untuk generate topup"
              on:change={handleStoreScopeChange}
            />

            {#if currentStore()}
              <div class="rounded-[1.7rem] border border-ink-100 bg-canvas-50 px-4 py-4 text-sm text-ink-700">
                <p class="font-semibold text-ink-900">{currentStore()?.name}</p>
                <p>Balance sekarang: {formatCurrency(currentStore()?.current_balance)}</p>
                <p>
                  Low balance threshold:
                  {currentStore()?.low_balance_threshold
                    ? formatCurrency(currentStore()?.low_balance_threshold)
                    : '-'}
                </p>
              </div>
            {/if}

            {#if hasLowBalanceStore()}
              <Notice
                tone="warning"
                title="Saldo toko sedang menipis"
                message="Store aktif sudah menyentuh low balance threshold. Topup ini bisa dipakai untuk mengisi buffer saldo sebelum deposit game atau withdraw berikutnya."
              />
            {/if}

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Nominal topup</span>
              <input
                bind:value={amountInput}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                inputmode="numeric"
                placeholder="50000"
              />
              <p class={`text-xs leading-5 ${amountError === '' ? 'text-ink-500' : 'text-rose-700'}`}>
                {amountInput.trim() === ''
                  ? 'Gunakan angka bulat tanpa titik atau koma, misalnya 50000.'
                  : amountError === ''
                    ? 'Nominal siap dikirim ke provider QRIS.'
                    : amountError}
              </p>
            </label>
          </div>

          <div class="mt-5">
            <Button variant="brand" size="lg" onclick={submitCreateTopup} disabled={busy || amountInput.trim() === '' || amountError !== ''}>
              Generate QRIS
            </Button>
          </div>

          <div class="mt-8 rounded-[1.9rem] border border-dashed border-ink-200 bg-white p-5">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="font-semibold text-ink-900">QR aktif</h3>
                <p class="mt-1 text-sm leading-6 text-ink-700">
                  {#if selectedTopup}
                    Ref: <span class="font-mono text-ink-900">{selectedTopup.custom_ref}</span>
                  {:else}
                    Belum ada topup terpilih.
                  {/if}
                </p>
              </div>

              {#if selectedTopup}
                <span class={`rounded-full border px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] ${statusClass(selectedTopup.status)}`}>
                  {statusLabel(selectedTopup.status)}
                </span>
              {/if}
            </div>

            {#if selectedTopup && selectedTopup.qr_code_value && qrCodeDataURL !== ''}
              <div class="mt-5 flex flex-col items-center gap-4">
                <img alt="QRIS topup" class="w-full max-w-[320px] rounded-[1.8rem] border border-ink-100 bg-white p-4" src={qrCodeDataURL} />
                <p class="text-center text-sm leading-6 text-ink-700">
                  Scan QR ini untuk menyelesaikan topup. History transaksi tetap tersedia walau owner
                  membuat beberapa pending topup sekaligus.
                </p>
              </div>
            {:else if selectedTopup}
              <div class="mt-5 rounded-3xl bg-canvas-50 px-4 py-4 text-sm leading-6 text-ink-700">
                {providerNote(selectedTopup)}
              </div>
            {:else}
              <div class="mt-5 rounded-3xl bg-canvas-50 px-4 py-4 text-sm text-ink-700">
                Belum ada transaksi topup untuk toko ini.
              </div>
            {/if}
          </div>
        </section>

        <section class="glass-panel rounded-4xl p-6">
          <div class="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <h2 class="font-display text-2xl font-bold text-ink-900">History QRIS</h2>
              <p class="mt-2 text-sm leading-6 text-ink-700">
                List ini ditarik langsung dari backend dengan pagination server-side, search, status
                filter, dan date range supaya histori tetap ringan saat row sudah besar.
              </p>
            </div>

            <div class="grid w-full gap-4 xl:max-w-[26rem] xl:grid-cols-[minmax(0,1fr)_14rem]">
              <article class="rounded-[1.7rem] bg-canvas-50 px-4 py-4">
                <p class="text-sm font-semibold text-ink-900">Status distribution</p>
                <ChartCanvas class="mt-4 h-[220px]" config={topupMixChart} />
              </article>
              <div class="flex items-start justify-end">
                <Button variant="outline" size="lg" onclick={() => loadTopups(selectedTopupID)} disabled={busy}>
                  Refresh
                </Button>
              </div>
            </div>
          </div>

          <div class="mt-5 grid gap-4 xl:grid-cols-[12rem_12rem_minmax(0,1fr)]">
            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Type</span>
              <select
                bind:value={transactionType}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
              >
                <option value="store_topup">Store topup</option>
                <option value="member_payment">Member payment</option>
              </select>
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Status</span>
              <select
                bind:value={statusFilter}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
              >
                <option value="all">Semua status</option>
                <option value="pending">Pending</option>
                <option value="success">Success</option>
                <option value="failed">Failed</option>
                <option value="expired">Expired</option>
              </select>
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Cari transaksi</span>
              <input
                bind:value={topupSearchTerm}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                placeholder="Cari custom ref, username, atau provider trx"
              />
            </label>
          </div>

          <div class="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(18rem,24rem)]">
            <DateRangeFilter bind:start={topupCreatedFrom} bind:end={topupCreatedTo} />
            <div class="grid gap-4">
              <div class="flex flex-wrap items-center justify-end gap-3">
                <Button variant="outline" size="lg" onclick={resetFilters} disabled={busy}>
                  Reset Filter
                </Button>
                <Button variant="brand" size="lg" onclick={applyFilters} disabled={busy}>
                  Apply Filter
                </Button>
              </div>

              <ExportActions
                count={topups.length}
                disabled={topups.length === 0}
                onCsv={exportTopupsToCSV}
                onXlsx={exportTopupsToXLSX}
                onPdf={exportTopupsToPDF}
              />
            </div>
          </div>

          <div class="mt-5 space-y-4">
            {#if topupTotalCount === 0}
              <EmptyState
                eyebrow="QRIS History"
                title="Belum ada histori QRIS"
                body={transactionType === 'store_topup'
                  ? 'Toko ini belum punya transaksi `store_topup`. Generate QR pertama akan langsung tampil di panel aktif dan histori di sisi kanan.'
                  : 'Belum ada histori `member_payment` untuk toko ini. Event akan muncul setelah website owner mulai memakai store API QRIS.'}
              />
            {:else if topups.length === 0}
              <EmptyState
                eyebrow="Filter Result"
                title="Tidak ada transaksi yang cocok"
                body={`Filter tipe ${transactionType}, status ${statusFilter}, kata kunci "${topupSearchTerm}", dan rentang waktu aktif belum menemukan transaksi yang sesuai.`}
              />
            {:else}
              {#each topups as topup}
                <button
                  class={`w-full rounded-[1.7rem] border p-4 text-left transition ${
                    selectedTopupID === topup.id
                      ? 'border-brand-300 bg-brand-100/40'
                      : 'border-ink-100 bg-white hover:border-accent-300 hover:bg-canvas-50'
                  }`}
                  onclick={() => {
                    selectedTopupID = topup.id;
                  }}
                  type="button"
                >
                  <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                    <div class="space-y-2">
                      <div class="flex flex-wrap items-center gap-2">
                        <span class={`rounded-full border px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] ${statusClass(topup.status)}`}>
                          {statusLabel(topup.status)}
                        </span>
                        <span class="rounded-full bg-brand-100 px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-brand-700">
                          {topup.type === 'member_payment' ? 'Member payment' : 'Store topup'}
                        </span>
                        <span class="rounded-full bg-canvas-100 px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-ink-700">
                          {topup.external_username}
                        </span>
                      </div>

                      <h3 class="font-semibold text-ink-900">{formatCurrency(topup.amount_gross)}</h3>
                      <p class="font-mono text-xs text-ink-700">{topup.custom_ref}</p>
                      <p class="text-sm leading-6 text-ink-700">{providerNote(topup)}</p>
                    </div>

                    <div class="rounded-[1.5rem] bg-canvas-100 px-4 py-3 text-sm text-ink-700">
                      <p>Created: {formatDateTime(topup.created_at)}</p>
                      <p>Expires: {topup.expires_at ? formatDateTime(topup.expires_at) : '-'}</p>
                      <p>Provider trx: {topup.provider_trx_id ?? '-'}</p>
                    </div>
                  </div>
                </button>
              {/each}
            {/if}
          </div>

          {#if topupTotalCount > 0}
            <div class="mt-5">
              <PaginationControls bind:page={topupPage} bind:pageSize={topupPageSize} totalItems={topupTotalCount} />
            </div>
          {/if}
        </section>
      </div>
    {/if}
  </div>
{/if}
