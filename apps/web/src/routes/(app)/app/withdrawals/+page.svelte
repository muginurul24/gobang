<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import type { ChartConfiguration } from 'chart.js';

  import ChartCanvas from '$lib/components/app/chart-canvas.svelte';
  import { chartTextColor as resolveChartTextColor } from '$lib/chart-theme';
  import DateRangeFilter from '$lib/components/app/date-range-filter.svelte';
  import EmptyState from '$lib/components/app/empty-state.svelte';
  import ExportActions from '$lib/components/app/export-actions.svelte';
  import MetricCard from '$lib/components/app/metric-card.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import PaginationControls from '$lib/components/app/pagination-controls.svelte';
  import PageSkeleton from '$lib/components/app/page-skeleton.svelte';
  import StoreScopePicker from '$lib/components/app/store-scope-picker.svelte';
  import Button from '$lib/components/ui/button/button.svelte';
  import { authSession, initializeAuthSession } from '$lib/auth/client';
  import { fetchBankAccounts, type BankAccount } from '$lib/bank-accounts/client';
  import { exportRowsToCSV, exportRowsToPDF, exportRowsToXLSX } from '$lib/exporters';
  import { formatCurrency, formatDateTime, formatNumber } from '$lib/formatters';
  import { isStoreLowBalance, type Store } from '$lib/stores/client';
  import {
    hydratePreferredStoreID,
    preferredStoreID,
    setPreferredStoreID
  } from '$lib/stores/preferences';
  import {
    createStoreWithdrawal,
    fetchStoreWithdrawals,
    type StoreWithdrawal
  } from '$lib/withdrawals/client';
  import { resolvedTheme } from '$lib/theme';

  let loading = true;
  let busy = false;
  let errorMessage = '';
  let successMessage = '';
  let storeScopeLoading = true;
  let storeScopeTotalCount = 0;
  let selectedStoreID = '';
  let selectedStore: Store | null = null;
  let bankAccounts: BankAccount[] = [];
  let selectedBankAccountID = '';
  let withdrawals: StoreWithdrawal[] = [];
  let withdrawalStatusFilter: StoreWithdrawal['status'] | 'all' = 'all';
  let amount = '';
  let idempotencyKey = newIdempotencyKey();
  let withdrawalSearchTerm = '';
  let withdrawalCreatedFrom = '';
  let withdrawalCreatedTo = '';
  let withdrawalPage = 1;
  let withdrawalPageSize = 6;
  let withdrawalTotalCount = 0;
  let lastWithdrawalPaginationKey = '';

  let pendingCount = 0;
  let successCount = 0;
  let failedCount = 0;
  let totalNet = 0;
  let totalPlatformFee = 0;
  let totalExternalFee = 0;
  $: chartTextColor = resolveChartTextColor($resolvedTheme);
  $: feeMixChart = buildFeeMixChart([totalPlatformFee, totalExternalFee]);

  $: amountError =
    amount.trim() !== '' && !(Number.isFinite(Number(amount)) && Number(amount) > 0)
      ? 'Nominal withdraw harus lebih besar dari nol.'
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
        await loadStoreData();
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

  $: if (!loading && selectedStoreID !== '') {
    const nextKey = `${selectedStoreID}:${withdrawalPage}:${withdrawalPageSize}`;
    if (nextKey !== lastWithdrawalPaginationKey) {
      lastWithdrawalPaginationKey = nextKey;
      void loadStoreData();
    }
  }

  async function loadStoreData() {
    if (selectedStoreID === '') {
      bankAccounts = [];
      withdrawals = [];
      withdrawalTotalCount = 0;
      pendingCount = 0;
      successCount = 0;
      failedCount = 0;
      totalNet = 0;
      totalPlatformFee = 0;
      totalExternalFee = 0;
      selectedBankAccountID = '';
      lastWithdrawalPaginationKey = '';
      return;
    }

    const [bankAccountsResponse, withdrawalsResponse] = await Promise.all([
      fetchBankAccounts(selectedStoreID),
      fetchStoreWithdrawals(selectedStoreID, {
        status: withdrawalStatusFilter,
        query: withdrawalSearchTerm,
        limit: withdrawalPageSize,
        offset: (withdrawalPage - 1) * withdrawalPageSize,
        createdFrom: withdrawalCreatedFrom,
        createdTo: withdrawalCreatedTo
      })
    ]);

    if (!(await ensureAuthorized(bankAccountsResponse.message))) {
      return;
    }
    if (!(await ensureAuthorized(withdrawalsResponse.message))) {
      return;
    }

    if (!bankAccountsResponse.status || bankAccountsResponse.message !== 'SUCCESS') {
      errorMessage = toMessage(bankAccountsResponse.message);
      bankAccounts = [];
    } else {
      bankAccounts = (bankAccountsResponse.data.items ?? []).filter((account) => account.is_active);
    }

    if (!withdrawalsResponse.status || withdrawalsResponse.message !== 'SUCCESS') {
      errorMessage = toMessage(withdrawalsResponse.message);
      withdrawals = [];
      withdrawalTotalCount = 0;
      pendingCount = 0;
      successCount = 0;
      failedCount = 0;
      totalNet = 0;
      totalPlatformFee = 0;
      totalExternalFee = 0;
    } else {
      withdrawals = withdrawalsResponse.data.items ?? [];
      withdrawalTotalCount = withdrawalsResponse.data.summary?.total_count ?? 0;
      pendingCount = withdrawalsResponse.data.summary?.pending_count ?? 0;
      successCount = withdrawalsResponse.data.summary?.success_count ?? 0;
      failedCount = withdrawalsResponse.data.summary?.failed_count ?? 0;
      totalNet = Number(withdrawalsResponse.data.summary?.total_net_amount ?? 0);
      totalPlatformFee = Number(withdrawalsResponse.data.summary?.total_platform_fee ?? 0);
      totalExternalFee = Number(withdrawalsResponse.data.summary?.total_external_fee ?? 0);
    }

    lastWithdrawalPaginationKey = `${selectedStoreID}:${withdrawalPage}:${withdrawalPageSize}`;

    if (
      selectedBankAccountID === '' ||
      !bankAccounts.some((bankAccount) => bankAccount.id === selectedBankAccountID)
    ) {
      selectedBankAccountID = bankAccounts[0]?.id ?? '';
    }
  }

  async function handleStoreScopeChange(event: CustomEvent<{ storeID: string; store: Store | null }>) {
    selectedStoreID = event.detail.storeID;
    selectedStore = event.detail.store;
    setPreferredStoreID(selectedStoreID);
    withdrawalPage = 1;
    lastWithdrawalPaginationKey = '';
    successMessage = '';
    errorMessage = '';
    await loadStoreData();
  }

  async function submitWithdrawal() {
    if (selectedStoreID === '' || selectedBankAccountID === '') {
      errorMessage = 'Pilih toko dan rekening aktif terlebih dahulu.';
      return;
    }

    if (amount.trim() === '') {
      errorMessage = 'Nominal withdraw wajib diisi.';
      return;
    }

    if (amountError !== '') {
      errorMessage = amountError;
      return;
    }
    const parsedAmount = Number(amount);

    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await createStoreWithdrawal(selectedStoreID, {
      bank_account_id: selectedBankAccountID,
      amount: Math.trunc(parsedAmount),
      idempotency_key: idempotencyKey
    });

    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      lastWithdrawalPaginationKey = '';
      await loadStoreData();
      errorMessage = toMessage(response.message);
      if (response.data?.status === 'failed') {
        successMessage = 'Intent withdraw direkam sebagai failed. Gunakan request key baru untuk mencoba lagi.';
      }
      idempotencyKey = newIdempotencyKey();
      return;
    }

    const createdWithdrawal = response.data;
    if (!createdWithdrawal) {
      errorMessage = 'Respons withdraw tidak lengkap.';
      idempotencyKey = newIdempotencyKey();
      return;
    }

    successMessage =
      createdWithdrawal.status === 'pending'
        ? 'Withdraw berhasil dibuat. Transfer menunggu finalisasi provider.'
        : 'Withdraw dengan request key ini sudah ada; menampilkan transaksi yang sama.';
    amount = '';
    withdrawalStatusFilter = 'all';
    withdrawalPage = 1;
    lastWithdrawalPaginationKey = '';
    idempotencyKey = newIdempotencyKey();
    await loadStoreData();
  }

  async function applyFilters() {
    withdrawalPage = 1;
    lastWithdrawalPaginationKey = '';
    await loadStoreData();
  }

  async function resetFilters() {
    withdrawalStatusFilter = 'all';
    withdrawalSearchTerm = '';
    withdrawalCreatedFrom = '';
    withdrawalCreatedTo = '';
    withdrawalPage = 1;
    lastWithdrawalPaginationKey = '';
    await loadStoreData();
  }

  function currentStore() {
    return selectedStore;
  }

  function selectedBankAccount() {
    return bankAccounts.find((bankAccount) => bankAccount.id === selectedBankAccountID) ?? null;
  }

  function canUseWithdrawals() {
    return ['owner', 'dev', 'superadmin'].includes($authSession?.user.role ?? '');
  }

  function hasLowBalanceStore() {
    const store = currentStore();
    return store ? isStoreLowBalance(store) : false;
  }

  function newIdempotencyKey() {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID();
    }

    return `withdraw-${Date.now()}`;
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
        return 'Role Anda tidak bisa mengakses withdraw dashboard.';
      case 'STORE_INACTIVE':
        return 'Store tidak aktif sehingga withdraw belum bisa dibuat.';
      case 'INVALID_AMOUNT':
        return 'Nominal withdraw harus berupa angka bulat positif.';
      case 'INVALID_IDEMPOTENCY_KEY':
        return 'Request key withdraw tidak valid.';
      case 'IDEMPOTENCY_KEY_CONFLICT':
        return 'Request key yang sama sudah dipakai untuk intent withdraw yang berbeda.';
      case 'BANK_ACCOUNT_INACTIVE':
        return 'Rekening tujuan sudah tidak aktif.';
      case 'INSUFFICIENT_STORE_BALANCE':
        return 'Saldo toko tidak cukup setelah platform fee dan external fee dihitung.';
      case 'WITHDRAW_INQUIRY_UNAVAILABLE':
        return 'Inquiry withdraw sedang tidak tersedia.';
      case 'WITHDRAW_INQUIRY_FAILED':
        return 'Provider menolak inquiry withdraw untuk rekening ini.';
      case 'WITHDRAW_TRANSFER_FAILED':
        return 'Transfer ditolak provider dan reserve saldo sudah dilepas kembali.';
      case 'NOT_FOUND':
        return 'Store atau rekening tujuan tidak ditemukan.';
      default:
        return 'Terjadi kesalahan. Coba ulangi.';
    }
  }

  function buildFeeMixChart(values: number[]): ChartConfiguration<'doughnut'> {
    return {
      type: 'doughnut',
      data: {
        labels: ['Platform fee', 'External fee'],
        datasets: [
          {
            data: values,
            backgroundColor: ['#efc86d', '#d66b5a'],
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

  function exportWithdrawalsToCSV() {
    exportRowsToCSV(
      `${selectedStoreID || 'store'}-withdrawals`,
      [
        { label: 'Status', value: (withdrawal) => withdrawal.status },
        { label: 'Bank Code', value: (withdrawal) => withdrawal.bank_code },
        { label: 'Bank Name', value: (withdrawal) => withdrawal.bank_name },
        { label: 'Account Name', value: (withdrawal) => withdrawal.account_name },
        { label: 'Account Number', value: (withdrawal) => withdrawal.account_number_masked },
        { label: 'Net Requested', value: (withdrawal) => withdrawal.net_requested_amount },
        { label: 'Platform Fee', value: (withdrawal) => withdrawal.platform_fee_amount },
        { label: 'External Fee', value: (withdrawal) => withdrawal.external_fee_amount },
        { label: 'Total Store Debit', value: (withdrawal) => withdrawal.total_store_debit },
        { label: 'Provider Ref', value: (withdrawal) => withdrawal.provider_partner_ref_no ?? '-' },
        { label: 'Created At', value: (withdrawal) => formatDateTime(withdrawal.created_at) }
      ],
      withdrawals,
    );
  }

  function exportWithdrawalsToXLSX() {
    exportRowsToXLSX(
      `${selectedStoreID || 'store'}-withdrawals`,
      'Withdrawals',
      [
        { label: 'Status', value: (withdrawal) => withdrawal.status },
        { label: 'Bank Code', value: (withdrawal) => withdrawal.bank_code },
        { label: 'Bank Name', value: (withdrawal) => withdrawal.bank_name },
        { label: 'Account Name', value: (withdrawal) => withdrawal.account_name },
        { label: 'Account Number', value: (withdrawal) => withdrawal.account_number_masked },
        { label: 'Net Requested', value: (withdrawal) => withdrawal.net_requested_amount },
        { label: 'Platform Fee', value: (withdrawal) => withdrawal.platform_fee_amount },
        { label: 'External Fee', value: (withdrawal) => withdrawal.external_fee_amount },
        { label: 'Total Store Debit', value: (withdrawal) => withdrawal.total_store_debit },
        { label: 'Provider Ref', value: (withdrawal) => withdrawal.provider_partner_ref_no ?? '-' },
        { label: 'Created At', value: (withdrawal) => formatDateTime(withdrawal.created_at) }
      ],
      withdrawals,
    );
  }

  function exportWithdrawalsToPDF() {
    exportRowsToPDF(
      `${selectedStoreID || 'store'}-withdrawals`,
      'Store Withdrawal History',
      [
        { label: 'Status', value: (withdrawal) => withdrawal.status },
        { label: 'Bank', value: (withdrawal) => `${withdrawal.bank_code} ${withdrawal.bank_name}` },
        { label: 'Account', value: (withdrawal) => withdrawal.account_name },
        { label: 'Net', value: (withdrawal) => formatCurrency(withdrawal.net_requested_amount) },
        { label: 'Total Debit', value: (withdrawal) => formatCurrency(withdrawal.total_store_debit) },
        { label: 'Provider Ref', value: (withdrawal) => withdrawal.provider_partner_ref_no ?? '-' },
        { label: 'Created', value: (withdrawal) => formatDateTime(withdrawal.created_at) }
      ],
      withdrawals,
    );
  }
</script>

<svelte:head>
  <title>Withdrawals | onixggr</title>
</svelte:head>

{#if loading}
  <PageSkeleton blocks={4} />
{:else}
  <div class="space-y-6">
    <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
      <div class="flex flex-col gap-5 md:flex-row md:items-start md:justify-between">
        <div class="space-y-2">
          <p class="section-kicker">
            Store Withdrawal
          </p>
          <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
            Payout desk untuk withdraw saldo toko ke rekening bank.
          </h1>
          <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
            Flow ini menjalankan inquiry dulu, menghitung platform fee 12% + external fee, lalu
            reserve saldo toko sebelum transfer. Setiap submit dashboard membawa `idempotency_key`
            agar double click atau retry browser tidak membuat transfer ganda.
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
      <Notice tone="error" title="Withdraw belum bisa dibuat" message={errorMessage} />
    {/if}

    {#if successMessage}
      <Notice tone="success" title="Withdraw tersimpan" message={successMessage} />
    {/if}

    {#if !canUseWithdrawals()}
      <EmptyState
        eyebrow="Role Scope"
        title="Role ini tidak bisa memakai withdrawal dashboard"
        body="Withdraw dashboard hanya tersedia untuk owner, dev, dan superadmin agar approval payout tidak keluar dari jalur yang benar."
      />
    {:else if storeScopeLoading}
      <PageSkeleton blocks={2} />
    {:else if storeScopeTotalCount === 0}
      <EmptyState
        eyebrow="Store Withdrawal"
        title="Belum ada toko untuk withdraw"
        body="Tambahkan toko lebih dulu atau pastikan toko yang relevan memang ada di scope sesi dashboard ini."
        actionHref="/app/stores"
        actionLabel="Buka Stores"
      />
    {:else}
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          eyebrow="Queue"
          title="Pending withdraw"
          value={formatNumber(pendingCount)}
          detail="Masih menunggu webhook provider atau status checker."
          tone="accent"
        />
        <MetricCard
          eyebrow="Success"
          title="Settled withdraw"
          value={formatNumber(successCount)}
          detail="Withdraw yang sudah final sukses pada store aktif."
          tone="brand"
        />
        <MetricCard
          eyebrow="Requested"
          title="Net requested"
          value={formatCurrency(totalNet)}
          detail="Akumulasi nominal bersih yang diminta di histori store aktif."
        />
        <MetricCard
          eyebrow="Fees"
          title="Fee captured"
          value={formatCurrency(totalPlatformFee + totalExternalFee)}
          detail="Akumulasi platform fee dan external fee di histori store aktif."
          tone="accent"
        />
      </div>

      <div class="grid gap-6 xl:grid-cols-[1.05fr_0.95fr]">
        <section class="glass-panel rounded-4xl p-6">
          <h2 class="font-display text-2xl font-bold text-ink-900">Create withdrawal</h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            Pilih toko, pilih rekening aktif, lalu input nominal bersih yang ingin diterima owner.
          </p>

          <div class="mt-5 space-y-4">
            <StoreScopePicker
              bind:selectedStoreID
              bind:selectedStore
              bind:loading={storeScopeLoading}
              bind:totalCount={storeScopeTotalCount}
              compact
              title="Store scope untuk withdrawal"
              description="Withdrawal desk memakai directory backend yang dipaginasi, sehingga selector store tetap ringan saat volume toko besar."
              placeholder="Cari store untuk payout"
              on:change={handleStoreScopeChange}
            />

            {#if currentStore()}
              <div class="rounded-[1.7rem] border border-ink-100 bg-canvas-50 px-4 py-4 text-sm text-ink-700">
                <p class="font-semibold text-ink-900">Current balance</p>
                <p class="mt-1 font-mono text-base text-ink-900">{formatCurrency(currentStore()?.current_balance)}</p>
                <p class="mt-2 text-xs uppercase tracking-[0.24em] text-ink-500">
                  Request key {idempotencyKey.slice(0, 12)}...
                </p>
              </div>
            {/if}

            {#if hasLowBalanceStore()}
              <Notice
                tone="warning"
                title="Store aktif sedang low balance"
                message="Saldo store aktif sudah menyentuh threshold. Pastikan nominal withdraw tidak memotong buffer operasional yang masih dibutuhkan."
              />
            {/if}

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Rekening tujuan aktif</span>
              <select
                bind:value={selectedBankAccountID}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
              >
                {#if bankAccounts.length === 0}
                  <option value="">Belum ada rekening aktif</option>
                {:else}
                  {#each bankAccounts as bankAccount}
                    <option value={bankAccount.id}>
                      {bankAccount.bank_code} · {bankAccount.bank_name} · {bankAccount.account_number_masked}
                    </option>
                  {/each}
                {/if}
              </select>
            </label>

            {#if selectedBankAccount()}
              <div class="rounded-[1.7rem] border border-ink-100 bg-white px-4 py-4 text-sm text-ink-700">
                <p class="font-semibold text-ink-900">{selectedBankAccount()?.bank_name}</p>
                <p class="mt-1">{selectedBankAccount()?.account_name}</p>
                <p class="font-mono text-ink-900">{selectedBankAccount()?.account_number_masked}</p>
              </div>
            {/if}

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Net amount</span>
              <input
                bind:value={amount}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                inputmode="numeric"
                placeholder="1000000"
              />
              <p class={`text-xs leading-5 ${amountError === '' ? 'text-ink-500' : 'text-rose-700'}`}>
                {amount.trim() === ''
                  ? 'Nominal ini adalah jumlah bersih yang ingin diterima owner, sebelum fee tampil di riwayat.'
                  : amountError === ''
                    ? 'Nominal withdraw terlihat valid untuk diproses.'
                    : amountError}
              </p>
            </label>
          </div>

          <div class="mt-5 flex flex-wrap gap-3">
            <Button
              variant="brand"
              size="lg"
              onclick={submitWithdrawal}
              disabled={busy || bankAccounts.length === 0 || amount.trim() === '' || amountError !== ''}
            >
              Submit Withdrawal
            </Button>
            <Button
              variant="outline"
              size="lg"
              onclick={() => {
                idempotencyKey = newIdempotencyKey();
              }}
              disabled={busy}
            >
              New Request Key
            </Button>
          </div>
        </section>

        <section class="glass-panel rounded-4xl p-6">
          <div class="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <h2 class="font-display text-2xl font-bold text-ink-900">Withdrawal history</h2>
              <p class="mt-2 text-sm leading-6 text-ink-700">
                Riwayat request withdraw per toko. Tabel ini dipaginate di server agar tetap cepat
                saat histori payout membesar, sementara final status tetap diputuskan webhook
                transfer dan status checker.
              </p>
            </div>

            <article class="w-full rounded-[1.7rem] bg-canvas-50 px-4 py-4 lg:max-w-[20rem]">
              <p class="text-sm font-semibold text-ink-900">Fee mix</p>
              <ChartCanvas class="mt-4 h-[220px]" config={feeMixChart} />
            </article>
          </div>

          <div class="mt-5 grid gap-4 md:grid-cols-[12rem_minmax(0,1fr)]">
            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Status</span>
              <select
                bind:value={withdrawalStatusFilter}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
              >
                <option value="all">Semua status</option>
                <option value="pending">Pending</option>
                <option value="success">Success</option>
                <option value="failed">Failed</option>
              </select>
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Cari withdraw</span>
              <input
                bind:value={withdrawalSearchTerm}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                placeholder="Cari bank, account name, atau provider ref"
              />
            </label>
          </div>

          <div class="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(18rem,24rem)]">
            <DateRangeFilter bind:start={withdrawalCreatedFrom} bind:end={withdrawalCreatedTo} />
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
                count={withdrawals.length}
                disabled={withdrawals.length === 0}
                onCsv={exportWithdrawalsToCSV}
                onXlsx={exportWithdrawalsToXLSX}
                onPdf={exportWithdrawalsToPDF}
              />
            </div>
          </div>

          <div class="mt-5 space-y-4">
            {#if withdrawalTotalCount === 0}
              <EmptyState
                eyebrow="Withdrawal History"
                title="Belum ada request withdraw"
                body="Request withdraw yang berhasil dibuat akan muncul di sini bersama status pending, success, atau failed."
              />
            {:else if withdrawals.length === 0}
              <EmptyState
                eyebrow="Filter Result"
                title="Tidak ada withdraw yang cocok"
                body={`Filter status ${withdrawalStatusFilter}, kata kunci "${withdrawalSearchTerm}", dan rentang waktu aktif belum menemukan hasil.`}
              />
            {:else}
              {#each withdrawals as withdrawal}
                <article class="rounded-[1.7rem] border border-ink-100 bg-white p-4 shadow-[0_16px_34px_rgba(7,16,12,0.08)]">
                  <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                    <div class="space-y-1">
                      <p class="text-xs font-semibold uppercase tracking-[0.24em] text-accent-700">
                        {withdrawal.bank_code}
                      </p>
                      <h3 class="font-semibold text-ink-900">{withdrawal.bank_name}</h3>
                      <p class="text-sm text-ink-700">{withdrawal.account_name}</p>
                      <p class="font-mono text-sm text-ink-900">{withdrawal.account_number_masked}</p>
                    </div>

                    <div class="rounded-[1.5rem] bg-canvas-100 px-4 py-3 text-sm text-ink-700">
                      <p class="font-semibold uppercase text-ink-900">{withdrawal.status}</p>
                      <p>Net: {formatCurrency(withdrawal.net_requested_amount)}</p>
                      <p>Total debit: {formatCurrency(withdrawal.total_store_debit)}</p>
                    </div>
                  </div>

                  <div class="mt-4 grid gap-3 text-sm text-ink-700 md:grid-cols-3">
                    <div class="rounded-[1.5rem] border border-ink-100 bg-canvas-50 px-4 py-3">
                      <p class="font-semibold text-ink-900">Platform fee</p>
                      <p>{formatCurrency(withdrawal.platform_fee_amount)}</p>
                    </div>
                    <div class="rounded-[1.5rem] border border-ink-100 bg-canvas-50 px-4 py-3">
                      <p class="font-semibold text-ink-900">External fee</p>
                      <p>{formatCurrency(withdrawal.external_fee_amount)}</p>
                    </div>
                    <div class="rounded-[1.5rem] border border-ink-100 bg-canvas-50 px-4 py-3">
                      <p class="font-semibold text-ink-900">Created</p>
                      <p>{formatDateTime(withdrawal.created_at)}</p>
                    </div>
                  </div>
                </article>
              {/each}
            {/if}
          </div>

          {#if withdrawalTotalCount > 0}
            <div class="mt-5">
              <PaginationControls bind:page={withdrawalPage} bind:pageSize={withdrawalPageSize} totalItems={withdrawalTotalCount} />
            </div>
          {/if}
        </section>
      </div>
    {/if}
  </div>
{/if}
