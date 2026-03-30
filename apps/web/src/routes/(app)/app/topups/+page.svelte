<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import QRCode from 'qrcode';

  import { authSession, initializeAuthSession } from '$lib/auth/client';
  import EmptyState from '$lib/components/app/empty-state.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import PageSkeleton from '$lib/components/app/page-skeleton.svelte';
  import Button from '$lib/components/ui/button/button.svelte';
  import {
    createStoreTopup,
    fetchStoreTopups,
    type StoreTopup
  } from '$lib/payments-qris/client';
  import { fetchStores, isStoreLowBalance, type Store } from '$lib/stores/client';
  import {
    hydratePreferredStoreID,
    pickPreferredStoreID,
    preferredStoreID,
    setPreferredStoreID
  } from '$lib/stores/preferences';

  let loading = true;
  let busy = false;
  let errorMessage = '';
  let successMessage = '';
  let stores: Store[] = [];
  let selectedStoreID = '';
  let topups: StoreTopup[] = [];
  let selectedTopupID = '';
  let statusFilter: StoreTopup['status'] | 'all' = 'all';
  let amountInput = '';
  let topupSearchTerm = '';
  let qrCodeDataURL = '';
  let qrRequestID = 0;
  let selectedTopup: StoreTopup | null = null;

  $: normalizedTopupSearch = topupSearchTerm.trim().toLowerCase();
  $: amountError =
    amountInput.trim() !== '' && !/^[1-9][0-9]*$/.test(amountInput.trim())
      ? 'Nominal topup harus angka bulat lebih dari nol.'
      : '';
  $: filteredTopups = topups.filter((topup) => {
    const matchesStatus = statusFilter === 'all' || topup.status === statusFilter;
    const matchesSearch =
      normalizedTopupSearch === '' ||
      topup.custom_ref.toLowerCase().includes(normalizedTopupSearch) ||
      topup.external_username.toLowerCase().includes(normalizedTopupSearch) ||
      (topup.provider_trx_id ?? '').toLowerCase().includes(normalizedTopupSearch);

    return matchesStatus && matchesSearch;
  });

  onMount(() => {
    let active = true;
    hydratePreferredStoreID();
    const unsubscribe = preferredStoreID.subscribe(async (storeID) => {
      if (!active || loading || stores.length === 0) {
        return;
      }

      if (storeID !== '' && storeID !== selectedStoreID && stores.some((store) => store.id === storeID)) {
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

      await loadStoresAndTopups();
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

  async function loadStoresAndTopups() {
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
      topups = [];
      selectedTopupID = '';
      loading = false;
      return;
    }

    selectedStoreID = pickPreferredStoreID(stores, selectedStoreID);
    setPreferredStoreID(selectedStoreID);

    await loadTopups();
    loading = false;
  }

  async function loadTopups(preferredTopupID = '') {
    if (selectedStoreID === '') {
      topups = [];
      selectedTopupID = '';
      return;
    }

    const response = await fetchStoreTopups(selectedStoreID);
    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      topups = [];
      selectedTopupID = '';
      return;
    }

    topups = response.data ?? [];

    if (preferredTopupID !== '' && topups.some((topup) => topup.id === preferredTopupID)) {
      selectedTopupID = preferredTopupID;
      return;
    }

    selectedTopupID = pickPreferredTopup(topups)?.id ?? '';
  }

  async function changeStore(storeID: string) {
    selectedStoreID = storeID;
    setPreferredStoreID(selectedStoreID);
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
    await loadTopups(response.data.id);
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
    return stores.find((store) => store.id === selectedStoreID) ?? null;
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
</script>

<svelte:head>
  <title>Topups | onixggr</title>
</svelte:head>

{#if loading}
  <PageSkeleton blocks={5} />
{:else}
  <div class="space-y-6">
    <section class="glass-panel rounded-4xl p-6">
      <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div class="space-y-2">
          <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
            Store Topup QRIS
          </p>
          <h1 class="font-display text-3xl font-bold tracking-tight text-ink-900">
            Tambah saldo toko via QRIS dashboard
          </h1>
          <p class="max-w-3xl text-sm leading-6 text-ink-700">
            Hari 24 hanya menyelesaikan generate `store_topup` QRIS. Dashboard ini membuat
            transaksi pending, menyimpan `provider_trx_id`, merender QR image, dan menampilkan
            histori pending, success, failed, atau expired per toko.
          </p>
        </div>

        <div class="rounded-3xl bg-canvas-100 px-4 py-3 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Scope</p>
          <p>Role: {$authSession?.user.role ?? '-'}</p>
          <p>Toko tersedia: {stores.length}</p>
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
    {:else if stores.length === 0}
      <EmptyState
        eyebrow="Store Topup"
        title="Belum ada toko untuk di-topup"
        body="Tambahkan toko lebih dulu atau pastikan store yang relevan memang ada di scope sesi dashboard ini."
        actionHref="/app/stores"
        actionLabel="Buka Stores"
      />
    {:else}
      <div class="grid gap-6 xl:grid-cols-[0.95fr_1.05fr]">
        <section class="glass-panel rounded-4xl p-6">
          <h2 class="font-display text-2xl font-bold text-ink-900">Buat topup baru</h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            Provider generate akan memakai `username` owner store, `custom_ref` internal topup,
            dan `uuid` global dari konfigurasi QRIS.
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
              <div class="rounded-3xl border border-ink-100 bg-canvas-50 px-4 py-4 text-sm text-ink-700">
                <p class="font-semibold text-ink-900">{currentStore()?.name}</p>
                <p>Balance sekarang: {currentStore()?.current_balance}</p>
                <p>Low balance threshold: {currentStore()?.low_balance_threshold ?? '-'}</p>
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

          <div class="mt-8 rounded-3xl border border-dashed border-ink-200 bg-white p-5">
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
                <img alt="QRIS topup" class="w-full max-w-[320px] rounded-3xl border border-ink-100 bg-white p-4" src={qrCodeDataURL} />
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
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="font-display text-2xl font-bold text-ink-900">History topup</h2>
              <p class="mt-2 text-sm leading-6 text-ink-700">
                List ini memisahkan status pending, success, failed, dan expired untuk transaksi
                `store_topup`.
              </p>
            </div>

            <Button variant="outline" size="lg" onclick={() => loadTopups(selectedTopupID)} disabled={busy}>
              Refresh
            </Button>
          </div>

          <div class="mt-5 grid gap-4 md:grid-cols-[12rem_minmax(0,1fr)]">
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

          <div class="mt-5 space-y-4">
            {#if topups.length === 0}
              <EmptyState
                eyebrow="Topup History"
                title="Belum ada histori topup"
                body="Toko ini belum punya transaksi `store_topup`. Generate QR pertama akan langsung tampil di panel aktif dan histori di sisi kanan."
              />
            {:else if filteredTopups.length === 0}
              <EmptyState
                eyebrow="Filter Result"
                title="Tidak ada topup yang cocok"
                body={`Filter status ${statusFilter} dan kata kunci "${topupSearchTerm}" tidak menemukan transaksi yang sesuai.`}
              />
            {:else}
              {#each filteredTopups as topup}
                <button
                  class={`w-full rounded-3xl border p-4 text-left transition ${
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
                        <span class="rounded-full bg-canvas-100 px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-ink-700">
                          {topup.external_username}
                        </span>
                      </div>

                      <h3 class="font-semibold text-ink-900">{topup.amount_gross}</h3>
                      <p class="font-mono text-xs text-ink-700">{topup.custom_ref}</p>
                      <p class="text-sm leading-6 text-ink-700">{providerNote(topup)}</p>
                    </div>

                    <div class="rounded-3xl bg-canvas-100 px-4 py-3 text-sm text-ink-700">
                      <p>
                        Created:
                        {new Date(topup.created_at).toLocaleString('id-ID')}
                      </p>
                      <p>
                        Expires:
                        {topup.expires_at ? new Date(topup.expires_at).toLocaleString('id-ID') : '-'}
                      </p>
                      <p>Provider trx: {topup.provider_trx_id ?? '-'}</p>
                    </div>
                  </div>
                </button>
              {/each}
            {/if}
          </div>
        </section>
      </div>
    {/if}
  </div>
{/if}
