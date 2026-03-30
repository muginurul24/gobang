<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import Button from '$lib/components/ui/button/button.svelte';
  import { authSession, hydrateAuthSession } from '$lib/auth/client';
  import { fetchBankAccounts, type BankAccount } from '$lib/bank-accounts/client';
  import { fetchStores, type Store } from '$lib/stores/client';
  import {
    createStoreWithdrawal,
    fetchStoreWithdrawals,
    type StoreWithdrawal
  } from '$lib/withdrawals/client';

  let loading = true;
  let busy = false;
  let errorMessage = '';
  let successMessage = '';
  let stores: Store[] = [];
  let selectedStoreID = '';
  let bankAccounts: BankAccount[] = [];
  let selectedBankAccountID = '';
  let withdrawals: StoreWithdrawal[] = [];
  let amount = '';
  let idempotencyKey = newIdempotencyKey();

  onMount(async () => {
    hydrateAuthSession();

    if (!$authSession) {
      await goto('/login');
      return;
    }

    await loadPage();
  });

  async function loadPage() {
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

    stores = storesResponse.data;
    if (stores.length === 0) {
      selectedStoreID = '';
      bankAccounts = [];
      withdrawals = [];
      loading = false;
      return;
    }

    if (selectedStoreID === '' || !stores.some((store) => store.id === selectedStoreID)) {
      selectedStoreID = stores[0].id;
    }

    await loadStoreData();
    loading = false;
  }

  async function loadStoreData() {
    if (selectedStoreID === '') {
      bankAccounts = [];
      withdrawals = [];
      selectedBankAccountID = '';
      return;
    }

    const [bankAccountsResponse, withdrawalsResponse] = await Promise.all([
      fetchBankAccounts(selectedStoreID),
      fetchStoreWithdrawals(selectedStoreID)
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
      bankAccounts = bankAccountsResponse.data.filter((account) => account.is_active);
    }

    if (!withdrawalsResponse.status || withdrawalsResponse.message !== 'SUCCESS') {
      errorMessage = toMessage(withdrawalsResponse.message);
      withdrawals = [];
    } else {
      withdrawals = withdrawalsResponse.data;
    }

    if (
      selectedBankAccountID === '' ||
      !bankAccounts.some((bankAccount) => bankAccount.id === selectedBankAccountID)
    ) {
      selectedBankAccountID = bankAccounts[0]?.id ?? '';
    }
  }

  async function changeStore(storeID: string) {
    selectedStoreID = storeID;
    successMessage = '';
    errorMessage = '';
    await loadStoreData();
  }

  async function submitWithdrawal() {
    if (selectedStoreID === '' || selectedBankAccountID === '') {
      errorMessage = 'Pilih toko dan rekening aktif terlebih dahulu.';
      return;
    }

    const parsedAmount = Number(amount);
    if (!Number.isFinite(parsedAmount) || parsedAmount <= 0) {
      errorMessage = 'Nominal withdraw harus lebih besar dari nol.';
      return;
    }

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

    await loadStoreData();

    if (!response.status || response.message !== 'SUCCESS') {
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
    idempotencyKey = newIdempotencyKey();
  }

  function currentStore() {
    return stores.find((store) => store.id === selectedStoreID) ?? null;
  }

  function selectedBankAccount() {
    return bankAccounts.find((bankAccount) => bankAccount.id === selectedBankAccountID) ?? null;
  }

  function canUseWithdrawals() {
    return ['owner', 'dev', 'superadmin'].includes($authSession?.user.role ?? '');
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
</script>

<svelte:head>
  <title>Withdrawals | onixggr</title>
</svelte:head>

{#if loading}
  <div class="glass-panel rounded-[2rem] p-6">
    <p class="text-sm text-ink-700">Memuat formulir withdraw balance toko...</p>
  </div>
{:else}
  <div class="space-y-6">
    <section class="glass-panel rounded-[2rem] p-6">
      <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div class="space-y-2">
          <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
            Store Withdrawal
          </p>
          <h1 class="font-display text-3xl font-bold tracking-tight text-ink-900">
            Withdraw balance toko ke rekening bank
          </h1>
          <p class="max-w-3xl text-sm leading-6 text-ink-700">
            Flow ini menjalankan inquiry dulu, menghitung platform fee 12% + external fee, lalu
            reserve saldo toko sebelum transfer. Setiap submit dashboard membawa `idempotency_key`
            agar double click atau retry browser tidak membuat transfer ganda.
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

    {#if !canUseWithdrawals()}
      <div class="glass-panel rounded-[2rem] p-6 text-sm text-ink-700">
        Role ini tidak bisa melihat atau membuat withdraw toko.
      </div>
    {:else if stores.length === 0}
      <div class="glass-panel rounded-[2rem] p-6 text-sm text-ink-700">
        Belum ada toko yang bisa dipakai untuk withdraw.
      </div>
    {:else}
      <div class="grid gap-6 xl:grid-cols-[1.05fr_0.95fr]">
        <section class="glass-panel rounded-[2rem] p-6">
          <h2 class="font-display text-2xl font-bold text-ink-900">Create withdrawal</h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            Pilih toko, pilih rekening aktif, lalu input nominal bersih yang ingin diterima owner.
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

            {#if currentStore()}
              <div class="rounded-[1.5rem] border border-ink-100 bg-canvas-50 px-4 py-4 text-sm text-ink-700">
                <p class="font-semibold text-ink-900">Current balance</p>
                <p class="mt-1 font-mono text-base text-ink-900">
                  Rp {currentStore()?.current_balance}
                </p>
                <p class="mt-2 text-xs uppercase tracking-[0.24em] text-ink-500">
                  Request key {idempotencyKey.slice(0, 12)}...
                </p>
              </div>
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
              <div class="rounded-[1.5rem] border border-ink-100 bg-white px-4 py-4 text-sm text-ink-700">
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
            </label>
          </div>

          <div class="mt-5 flex flex-wrap gap-3">
            <Button variant="brand" size="lg" onclick={submitWithdrawal} disabled={busy || bankAccounts.length === 0}>
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

        <section class="glass-panel rounded-[2rem] p-6">
          <h2 class="font-display text-2xl font-bold text-ink-900">Withdrawal history</h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            Riwayat request withdraw per toko. Status final `success` atau `failed` akan dilanjutkan
            oleh milestone webhook dan check-status berikutnya.
          </p>

          <div class="mt-5 space-y-4">
            {#if withdrawals.length === 0}
              <div class="rounded-[1.5rem] border border-ink-100 bg-canvas-50 px-4 py-4 text-sm text-ink-700">
                Belum ada request withdraw untuk toko ini.
              </div>
            {:else}
              {#each withdrawals as withdrawal}
                <article class="rounded-[1.5rem] border border-ink-100 bg-white p-4">
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
                      <p>Net: Rp {withdrawal.net_requested_amount}</p>
                      <p>Total debit: Rp {withdrawal.total_store_debit}</p>
                    </div>
                  </div>

                  <div class="mt-4 grid gap-3 text-sm text-ink-700 md:grid-cols-3">
                    <div class="rounded-[1.5rem] border border-ink-100 bg-canvas-50 px-4 py-3">
                      <p class="font-semibold text-ink-900">Platform fee</p>
                      <p>Rp {withdrawal.platform_fee_amount}</p>
                    </div>
                    <div class="rounded-[1.5rem] border border-ink-100 bg-canvas-50 px-4 py-3">
                      <p class="font-semibold text-ink-900">External fee</p>
                      <p>Rp {withdrawal.external_fee_amount}</p>
                    </div>
                    <div class="rounded-[1.5rem] border border-ink-100 bg-canvas-50 px-4 py-3">
                      <p class="font-semibold text-ink-900">Created</p>
                      <p>{new Date(withdrawal.created_at).toLocaleString('id-ID')}</p>
                    </div>
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
