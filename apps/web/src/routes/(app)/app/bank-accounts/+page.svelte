<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

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
  import { exportRowsToCSV, exportRowsToPDF, exportRowsToXLSX } from '$lib/exporters';
  import { formatDateTime, formatNumber } from '$lib/formatters';
  import {
    createBankAccount,
    fetchBankAccounts,
    searchBanks,
    type BankAccount,
    type BankDirectoryEntry,
    updateBankAccountStatus
  } from '$lib/bank-accounts/client';
  import type { Store } from '$lib/stores/client';
  import {
    hydratePreferredStoreID,
    preferredStoreID,
    setPreferredStoreID
  } from '$lib/stores/preferences';

  let loading = true;
  let busy = false;
  let errorMessage = '';
  let successMessage = '';
  let storeScopeLoading = true;
  let storeScopeTotalCount = 0;
  let selectedStoreID = '';
  let selectedStore: Store | null = null;
  let bankAccounts: BankAccount[] = [];
  let totalBankAccountCount = 0;
  let activeAccountCount = 0;
  let inactiveAccountCount = 0;
  let bankQuery = '';
  let bankResults: BankDirectoryEntry[] = [];
  let selectedBank: BankDirectoryEntry | null = null;
  let accountNumber = '';
  let savedAccountSearchTerm = '';
  let savedAccountStatusFilter: 'all' | 'active' | 'inactive' = 'all';
  let savedAccountFrom = '';
  let savedAccountTo = '';
  let savedAccountPage = 1;
  let savedAccountPageSize = 6;
  let lastBankAccountQueryKey = '';

  $: bankQueryNote =
    bankQuery.trim() === ''
      ? 'Kosongkan untuk melihat shortlist bank awal, atau isi kode/nama bank spesifik.'
      : 'Pencarian mendukung bank code atau nama bank RTOL.';
  $: accountNumberError =
    accountNumber.trim() !== '' && !/^[0-9]{6,30}$/.test(accountNumber.trim())
      ? 'Nomor rekening harus numerik dengan panjang 6 sampai 30 digit.'
      : '';
  $: if (!loading && selectedStoreID !== '') {
    const nextKey = `${selectedStoreID}:${savedAccountPage}:${savedAccountPageSize}`;
    if (nextKey !== lastBankAccountQueryKey) {
      void loadBankAccounts();
    }
  }

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
        await loadBankAccounts();
      }
    });

    void (async () => {
      await initializeAuthSession();

      if (!$authSession) {
        await goto('/login');
        return;
      }

      await runBankSearch();
      loading = false;
    })();

    return () => {
      active = false;
      unsubscribe();
    };
  });

  async function loadBankAccounts() {
    totalBankAccountCount = 0;
    activeAccountCount = 0;
    inactiveAccountCount = 0;

    if (selectedStoreID === '') {
      bankAccounts = [];
      lastBankAccountQueryKey = '';
      return;
    }

    const response = await fetchBankAccounts(selectedStoreID, {
      query: savedAccountSearchTerm || undefined,
      status: savedAccountStatusFilter === 'all' ? undefined : savedAccountStatusFilter,
      limit: savedAccountPageSize,
      offset: (savedAccountPage - 1) * savedAccountPageSize,
      verifiedFrom: savedAccountFrom || undefined,
      verifiedTo: savedAccountTo || undefined
    });
    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      bankAccounts = [];
      return;
    }

    bankAccounts = response.data.items ?? [];
    totalBankAccountCount = response.data.summary?.total_count ?? 0;
    activeAccountCount = response.data.summary?.active_count ?? 0;
    inactiveAccountCount = response.data.summary?.inactive_count ?? 0;
    lastBankAccountQueryKey = `${selectedStoreID}:${savedAccountPage}:${savedAccountPageSize}`;
  }

  async function runBankSearch() {
    const response = await searchBanks(bankQuery, 15);
    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      bankResults = [];
      return;
    }

    bankResults = response.data ?? [];
  }

  async function handleStoreScopeChange(event: CustomEvent<{ storeID: string; store: Store | null }>) {
    selectedStoreID = event.detail.storeID;
    selectedStore = event.detail.store;
    setPreferredStoreID(selectedStoreID);
    savedAccountPage = 1;
    await loadBankAccounts();
  }

  async function submitCreateBankAccount() {
    if (selectedStoreID === '' || !selectedBank) {
      errorMessage = 'Pilih toko dan bank tujuan terlebih dahulu.';
      return;
    }

    if (accountNumber.trim() === '') {
      errorMessage = 'Nomor rekening wajib diisi sebelum inquiry.';
      return;
    }

    if (accountNumberError !== '') {
      errorMessage = accountNumberError;
      return;
    }

    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await createBankAccount(selectedStoreID, {
      bank_code: selectedBank.bank_code,
      account_number: accountNumber
    });
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    accountNumber = '';
    successMessage = 'Rekening berhasil diverifikasi dan disimpan.';
    await loadBankAccounts();
  }

  async function toggleStatus(bankAccount: BankAccount) {
    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await updateBankAccountStatus(
      selectedStoreID,
      bankAccount.id,
      !bankAccount.is_active
    );
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    successMessage = response.data.is_active
      ? 'Rekening diaktifkan kembali.'
      : 'Rekening dinonaktifkan.';
    await loadBankAccounts();
  }

  async function ensureAuthorized(message: string) {
    if (message !== 'UNAUTHORIZED') {
      return true;
    }

    await goto('/login');
    return false;
  }

  function currentStore() {
    return selectedStore;
  }

  function canUseBankAccounts() {
    return ['owner', 'dev', 'superadmin'].includes($authSession?.user.role ?? '');
  }

  function toMessage(message: string) {
    switch (message) {
      case 'FORBIDDEN':
        return 'Role Anda tidak bisa mengakses rekening tujuan withdraw.';
      case 'INVALID_BANK_CODE':
        return 'Bank code tidak valid terhadap daftar Bank RTOL.';
      case 'INVALID_ACCOUNT_NUMBER':
        return 'Nomor rekening harus numerik dengan panjang minimal yang valid.';
      case 'BANK_INQUIRY_FAILED':
        return 'Inquiry rekening gagal. Periksa nomor rekening atau bank code.';
      case 'BANK_INQUIRY_UNAVAILABLE':
        return 'Layanan inquiry rekening belum tersedia di environment ini.';
      case 'NOT_FOUND':
        return 'Store atau rekening yang diminta tidak ditemukan.';
      default:
        return 'Terjadi kesalahan. Coba ulangi.';
    }
  }

  function exportBankAccountsToCSV() {
    exportRowsToCSV(
      `${selectedStoreID || 'store'}-bank-accounts`,
      [
        { label: 'Status', value: (bankAccount) => (bankAccount.is_active ? 'Active' : 'Inactive') },
        { label: 'Bank Code', value: (bankAccount) => bankAccount.bank_code },
        { label: 'Bank Name', value: (bankAccount) => bankAccount.bank_name },
        { label: 'Account Name', value: (bankAccount) => bankAccount.account_name },
        { label: 'Account Number', value: (bankAccount) => bankAccount.account_number_masked },
        { label: 'Verified At', value: (bankAccount) => formatDateTime(bankAccount.verified_at) },
        { label: 'Created At', value: (bankAccount) => formatDateTime(bankAccount.created_at) }
      ],
      bankAccounts,
    );
  }

  function exportBankAccountsToXLSX() {
    exportRowsToXLSX(
      `${selectedStoreID || 'store'}-bank-accounts`,
      'BankAccounts',
      [
        { label: 'Status', value: (bankAccount) => (bankAccount.is_active ? 'Active' : 'Inactive') },
        { label: 'Bank Code', value: (bankAccount) => bankAccount.bank_code },
        { label: 'Bank Name', value: (bankAccount) => bankAccount.bank_name },
        { label: 'Account Name', value: (bankAccount) => bankAccount.account_name },
        { label: 'Account Number', value: (bankAccount) => bankAccount.account_number_masked },
        { label: 'Verified At', value: (bankAccount) => formatDateTime(bankAccount.verified_at) },
        { label: 'Created At', value: (bankAccount) => formatDateTime(bankAccount.created_at) }
      ],
      bankAccounts,
    );
  }

  function exportBankAccountsToPDF() {
    exportRowsToPDF(
      `${selectedStoreID || 'store'}-bank-accounts`,
      'Store Bank Accounts',
      [
        { label: 'Status', value: (bankAccount) => (bankAccount.is_active ? 'Active' : 'Inactive') },
        { label: 'Bank', value: (bankAccount) => `${bankAccount.bank_code} ${bankAccount.bank_name}` },
        { label: 'Account Name', value: (bankAccount) => bankAccount.account_name },
        { label: 'Masked Number', value: (bankAccount) => bankAccount.account_number_masked },
        { label: 'Verified', value: (bankAccount) => formatDateTime(bankAccount.verified_at) }
      ],
      bankAccounts,
    );
  }

  async function applySavedAccountFilters() {
    savedAccountPage = 1;
    await loadBankAccounts();
  }

  async function resetSavedAccountFilters() {
    savedAccountSearchTerm = '';
    savedAccountStatusFilter = 'all';
    savedAccountFrom = '';
    savedAccountTo = '';
    savedAccountPage = 1;
    await loadBankAccounts();
  }
</script>

<svelte:head>
  <title>Bank Accounts | onixggr</title>
</svelte:head>

{#if loading}
  <PageSkeleton blocks={4} />
{:else}
  <div class="space-y-6">
    <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
      <div class="flex flex-col gap-5 md:flex-row md:items-start md:justify-between">
        <div class="space-y-2">
          <p class="section-kicker">
            Withdrawal Destination
          </p>
          <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
            Directory rekening tujuan yang aman, masked, dan siap dipakai payout.
          </h1>
          <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
            Module ini memakai `docs/Bank RTOL.json` untuk validasi `bank_code`, lalu melakukan
            inquiry sebelum rekening disimpan. Nomor rekening penuh tidak ditampilkan lagi di UI;
            yang tampil hanya hasil masking.
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
      <Notice tone="error" title="Rekening belum bisa diproses" message={errorMessage} />
    {/if}

    {#if successMessage}
      <Notice tone="success" title="Rekening tersimpan" message={successMessage} />
    {/if}

    {#if !canUseBankAccounts()}
      <EmptyState
        eyebrow="Role Scope"
        title="Role ini tidak bisa mengelola rekening tujuan"
        body="Rekening withdraw hanya boleh diubah oleh owner, dev, dan superadmin agar data payout tidak bocor ke role yang tidak relevan."
      />
    {:else if storeScopeLoading}
      <PageSkeleton blocks={2} />
    {:else if storeScopeTotalCount === 0}
      <EmptyState
        eyebrow="Bank Accounts"
        title="Belum ada toko untuk dihubungkan"
        body="Tambahkan toko lebih dulu sebelum menyimpan rekening tujuan withdraw."
        actionHref="/app/stores"
        actionLabel="Buka Stores"
      />
    {:else}
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          eyebrow="Accounts"
          title="Stored accounts"
          value={formatNumber(totalBankAccountCount)}
          detail="Semua rekening payout terfilter di store aktif."
          tone="brand"
        />
        <MetricCard
          eyebrow="Accounts"
          title="Active"
          value={formatNumber(activeAccountCount)}
          detail="Rekening yang bisa dipakai flow withdraw saat ini."
        />
        <MetricCard
          eyebrow="Accounts"
          title="Inactive"
          value={formatNumber(inactiveAccountCount)}
          detail="Rekening yang masih tersimpan namun tidak aktif."
          tone={inactiveAccountCount > 0 ? 'accent' : 'default'}
        />
        <MetricCard
          eyebrow="Store"
          title="Current store"
          value={currentStore()?.name ?? '-'}
          detail="Semua operasi bank account mengikuti store aktif."
          tone="accent"
        />
      </div>

      <div class="grid gap-6 2xl:grid-cols-[1.05fr_0.95fr]">
        <section class="glass-panel rounded-4xl p-6">
          <h2 class="font-display text-2xl font-bold text-ink-900">Tambah rekening tujuan</h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            Pilih toko, cari bank berdasarkan `bank_code` atau nama bank, lalu masukkan nomor
            rekening untuk diverifikasi.
          </p>

          <div class="mt-5 space-y-4">
            <StoreScopePicker
              bind:selectedStoreID
              bind:selectedStore
              bind:loading={storeScopeLoading}
              bind:totalCount={storeScopeTotalCount}
              compact
              title="Store scope untuk rekening payout"
              description="Selector store memakai backend directory yang dipaginasi, agar cepat walau jumlah toko besar."
              placeholder="Cari store tujuan rekening withdraw"
              on:change={handleStoreScopeChange}
            />

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Cari bank RTOL</span>
              <div class="flex gap-3">
                <input
                  bind:value={bankQuery}
                  class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                  placeholder="contoh: 014, BCA, Bank Jago"
                />
                <Button variant="outline" size="lg" onclick={runBankSearch} disabled={busy}>
                  Cari
                </Button>
              </div>
              <p class="text-xs leading-5 text-ink-500">{bankQueryNote}</p>
            </label>

            <div class="rounded-3xl border border-ink-100 bg-white p-4">
              <p class="text-sm font-medium text-ink-900">Hasil bank</p>
              <div class="mt-3 space-y-2">
                {#if bankResults.length === 0}
                  <EmptyState
                    eyebrow="Bank Search"
                    title="Belum ada hasil bank"
                    body="Coba kode bank seperti 014 atau nama bank spesifik. Shortlist juga akan berubah saat Anda menekan tombol cari."
                  />
                {:else}
                  {#each bankResults as bank}
                    <button
                      class={`w-full rounded-2xl border px-4 py-3 text-left text-sm transition ${
                        selectedBank?.bank_code === bank.bank_code &&
                        selectedBank?.bank_name === bank.bank_name
                          ? 'border-brand-300 bg-brand-100/60 text-ink-900'
                          : 'border-ink-100 bg-canvas-50 text-ink-700 hover:border-accent-300 hover:bg-white'
                      }`}
                      onclick={() => {
                        selectedBank = bank;
                      }}
                      type="button"
                    >
                      <span class="block font-semibold text-ink-900">{bank.bank_code}</span>
                      <span class="block mt-1">{bank.bank_name}</span>
                    </button>
                  {/each}
                {/if}
              </div>
            </div>

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Nomor rekening</span>
              <input
                bind:value={accountNumber}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                inputmode="numeric"
                placeholder="100009689749"
              />
              <p class={`text-xs leading-5 ${accountNumberError === '' ? 'text-ink-500' : 'text-rose-700'}`}>
                {accountNumber.trim() === ''
                  ? 'Nomor rekening penuh hanya dipakai untuk inquiry dan disimpan terenkripsi.'
                  : accountNumberError === ''
                    ? 'Format nomor rekening terlihat valid untuk inquiry.'
                    : accountNumberError}
              </p>
            </label>
          </div>

          <div class="mt-5">
            <Button
              variant="brand"
              size="lg"
              onclick={submitCreateBankAccount}
              disabled={busy || accountNumber.trim() === '' || accountNumberError !== ''}
            >
              Verify and Save
            </Button>
          </div>
        </section>

        <section class="glass-panel rounded-4xl p-6">
          <h2 class="font-display text-2xl font-bold text-ink-900">Riwayat rekening toko</h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            {#if currentStore()}
              Rekening yang tersimpan untuk {currentStore()?.name}. Hanya nomor masked yang tampil di
              dashboard. Search, status, rentang waktu, dan pagination sekarang dieksekusi di
              backend agar tetap ringan saat row membesar.
            {/if}
          </p>

          <label class="mt-5 block space-y-2">
            <span class="text-sm font-medium text-ink-700">Filter rekening</span>
            <input
              bind:value={savedAccountSearchTerm}
              class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
              placeholder="Cari bank, account name, atau masked number"
            />
          </label>

          <div class="mt-4 grid gap-4 2xl:grid-cols-[12rem_minmax(0,1fr)]">
            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Status</span>
              <select
                bind:value={savedAccountStatusFilter}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
              >
                <option value="all">Semua status</option>
                <option value="active">Active</option>
                <option value="inactive">Inactive</option>
              </select>
            </label>

            <DateRangeFilter bind:start={savedAccountFrom} bind:end={savedAccountTo} label="Verified at" />
          </div>

          <div class="mt-4">
            <ExportActions
              count={bankAccounts.length}
              disabled={bankAccounts.length === 0}
              onCsv={exportBankAccountsToCSV}
              onXlsx={exportBankAccountsToXLSX}
              onPdf={exportBankAccountsToPDF}
            />
          </div>

          <div class="mt-4 flex flex-wrap gap-3">
            <Button variant="brand" size="lg" onclick={applySavedAccountFilters}>
              Apply Filters
            </Button>
            <Button variant="outline" size="lg" onclick={resetSavedAccountFilters}>
              Reset
            </Button>
          </div>

          <div class="mt-5 space-y-4">
            {#if totalBankAccountCount === 0}
              <EmptyState
                eyebrow="Stored Accounts"
                title="Belum ada rekening terverifikasi"
                body="Rekening yang lolos inquiry akan tampil di sini dalam bentuk masked agar aman untuk dashboard."
              />
            {:else if bankAccounts.length === 0}
              <EmptyState
                eyebrow="Filter Result"
                title="Tidak ada rekening yang cocok"
                body={`Tidak ada rekening yang cocok dengan filter "${savedAccountSearchTerm}" dan rentang waktu aktif.`}
              />
            {:else}
              {#each bankAccounts as bankAccount}
                <article class="rounded-3xl border border-ink-100 bg-white p-4">
                  <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                    <div class="space-y-1">
                      <p class="text-xs font-semibold uppercase tracking-[0.24em] text-accent-700">
                        {bankAccount.bank_code}
                      </p>
                      <h3 class="font-semibold text-ink-900">{bankAccount.bank_name}</h3>
                      <p class="text-sm text-ink-700">{bankAccount.account_name}</p>
                      <p class="font-mono text-sm text-ink-900">{bankAccount.account_number_masked}</p>
                    </div>

                    <div class="rounded-[1.5rem] bg-canvas-100 px-4 py-3 text-sm text-ink-700">
                      <p class="font-semibold text-ink-900">
                        {bankAccount.is_active ? 'Active' : 'Inactive'}
                      </p>
                      <p>Verified: {bankAccount.verified_at ? formatDateTime(bankAccount.verified_at) : '-'}</p>
                    </div>
                  </div>

                  <div class="mt-4">
                    <Button
                      variant="outline"
                      size="lg"
                      onclick={() => toggleStatus(bankAccount)}
                      disabled={busy}
                    >
                      {bankAccount.is_active ? 'Deactivate' : 'Activate'}
                    </Button>
                  </div>
                </article>
              {/each}
            {/if}
          </div>

          {#if totalBankAccountCount > 0}
            <div class="mt-5">
              <PaginationControls bind:page={savedAccountPage} bind:pageSize={savedAccountPageSize} totalItems={totalBankAccountCount} />
            </div>
          {/if}
        </section>
      </div>
    {/if}
  </div>
{/if}
