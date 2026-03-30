<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import Button from '$lib/components/ui/button/button.svelte';
  import { authSession, hydrateAuthSession } from '$lib/auth/client';
  import {
    createBankAccount,
    fetchBankAccounts,
    searchBanks,
    type BankAccount,
    type BankDirectoryEntry,
    updateBankAccountStatus
  } from '$lib/bank-accounts/client';
  import { fetchStores, type Store } from '$lib/stores/client';

  let loading = true;
  let busy = false;
  let errorMessage = '';
  let successMessage = '';
  let stores: Store[] = [];
  let selectedStoreID = '';
  let bankAccounts: BankAccount[] = [];
  let bankQuery = '';
  let bankResults: BankDirectoryEntry[] = [];
  let selectedBank: BankDirectoryEntry | null = null;
  let accountNumber = '';

  onMount(async () => {
    hydrateAuthSession();

    if (!$authSession) {
      await goto('/login');
      return;
    }

    await loadStoresAndAccounts();
  });

  async function loadStoresAndAccounts() {
    loading = true;
    errorMessage = '';

    const storesResponse = await fetchStores();
    if (!(await ensureAuthorized(storesResponse.message))) {
      return;
    }

    if (!storesResponse.status || storesResponse.message !== 'SUCCESS') {
      errorMessage = toMessage(storesResponse.message);
      loading = false;
      return;
    }

    stores = storesResponse.data ?? [];
    if (stores.length === 0) {
      selectedStoreID = '';
      bankAccounts = [];
      bankResults = [];
      loading = false;
      return;
    }

    if (selectedStoreID === '' || !stores.some((store) => store.id === selectedStoreID)) {
      selectedStoreID = stores[0].id;
    }

    await loadBankAccounts();
    await runBankSearch();
    loading = false;
  }

  async function loadBankAccounts() {
    if (selectedStoreID === '') {
      bankAccounts = [];
      return;
    }

    const response = await fetchBankAccounts(selectedStoreID);
    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      bankAccounts = [];
      return;
    }

    bankAccounts = response.data ?? [];
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

  async function changeStore(storeID: string) {
    selectedStoreID = storeID;
    await loadBankAccounts();
  }

  async function submitCreateBankAccount() {
    if (selectedStoreID === '' || !selectedBank) {
      errorMessage = 'Pilih toko dan bank tujuan terlebih dahulu.';
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
    return stores.find((store) => store.id === selectedStoreID) ?? null;
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
</script>

<svelte:head>
  <title>Bank Accounts | onixggr</title>
</svelte:head>

{#if loading}
  <div class="glass-panel rounded-[2rem] p-6">
    <p class="text-sm text-ink-700">Memuat daftar bank RTOL dan rekening tujuan withdraw...</p>
  </div>
{:else}
  <div class="space-y-6">
    <section class="glass-panel rounded-[2rem] p-6">
      <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div class="space-y-2">
          <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
            Withdrawal Destination
          </p>
          <h1 class="font-display text-3xl font-bold tracking-tight text-ink-900">
            Verifikasi rekening tujuan per toko
          </h1>
          <p class="max-w-3xl text-sm leading-6 text-ink-700">
            Module ini memakai `docs/Bank RTOL.json` untuk validasi `bank_code`, lalu melakukan
            inquiry sebelum rekening disimpan. Nomor rekening penuh tidak ditampilkan lagi di UI;
            yang tampil hanya hasil masking.
          </p>
        </div>

        <div class="rounded-[1.5rem] bg-canvas-100 px-4 py-3 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Scope</p>
          <p>Role: {$authSession?.user.role ?? '-'}</p>
          <p>Toko tersedia: {stores.length}</p>
        </div>
      </div>
    </section>

    {#if errorMessage}
      <div class="rounded-[1.5rem] border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
        {errorMessage}
      </div>
    {/if}

    {#if successMessage}
      <div class="rounded-[1.5rem] border border-brand-200 bg-brand-100/60 px-4 py-3 text-sm text-brand-700">
        {successMessage}
      </div>
    {/if}

    {#if !canUseBankAccounts()}
      <div class="glass-panel rounded-[2rem] p-6 text-sm text-ink-700">
        Role ini tidak bisa mengelola rekening tujuan withdraw.
      </div>
    {:else if stores.length === 0}
      <div class="glass-panel rounded-[2rem] p-6 text-sm text-ink-700">
        Belum ada toko untuk dihubungkan dengan rekening withdraw.
      </div>
    {:else}
      <div class="grid gap-6 xl:grid-cols-[1.05fr_0.95fr]">
        <section class="glass-panel rounded-[2rem] p-6">
          <h2 class="font-display text-2xl font-bold text-ink-900">Tambah rekening tujuan</h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            Pilih toko, cari bank berdasarkan `bank_code` atau nama bank, lalu masukkan nomor
            rekening untuk diverifikasi.
          </p>

          <div class="mt-5 space-y-4">
            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Toko</span>
              <select
                bind:value={selectedStoreID}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                onchange={(event) => changeStore((event.currentTarget as HTMLSelectElement).value)}
              >
                {#each stores as store}
                  <option value={store.id}>{store.name} · {store.slug}</option>
                {/each}
              </select>
            </label>

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
            </label>

            <div class="rounded-[1.5rem] border border-ink-100 bg-white p-4">
              <p class="text-sm font-medium text-ink-900">Hasil bank</p>
              <div class="mt-3 space-y-2">
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
            </label>
          </div>

          <div class="mt-5">
            <Button variant="brand" size="lg" onclick={submitCreateBankAccount} disabled={busy}>
              Verify and Save
            </Button>
          </div>
        </section>

        <section class="glass-panel rounded-[2rem] p-6">
          <h2 class="font-display text-2xl font-bold text-ink-900">Riwayat rekening toko</h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            {#if currentStore()}
              Rekening yang tersimpan untuk {currentStore()?.name}. Hanya nomor masked yang tampil di
              dashboard.
            {/if}
          </p>

          <div class="mt-5 space-y-4">
            {#if bankAccounts.length === 0}
              <div class="rounded-[1.5rem] border border-ink-100 bg-canvas-50 px-4 py-4 text-sm text-ink-700">
                Belum ada rekening terverifikasi untuk toko ini.
              </div>
            {:else}
              {#each bankAccounts as bankAccount}
                <article class="rounded-[1.5rem] border border-ink-100 bg-white p-4">
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
                      <p>
                        Verified:
                        {bankAccount.verified_at
                          ? new Date(bankAccount.verified_at).toLocaleString('id-ID')
                          : '-'}
                      </p>
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
        </section>
      </div>
    {/if}
  </div>
{/if}
