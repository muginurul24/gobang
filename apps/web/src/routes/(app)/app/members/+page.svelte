<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import Button from '$lib/components/ui/button/button.svelte';
  import { authSession, hydrateAuthSession } from '$lib/auth/client';
  import { createStoreMember, fetchStoreMembers, type StoreMember } from '$lib/store-members/client';
  import { fetchStores, type Store } from '$lib/stores/client';

  let loading = true;
  let busy = false;
  let errorMessage = '';
  let successMessage = '';
  let stores: Store[] = [];
  let members: StoreMember[] = [];
  let selectedStoreID = '';
  let createForm = {
    real_username: ''
  };

  onMount(async () => {
    hydrateAuthSession();

    if (!$authSession) {
      await goto('/login');
      return;
    }

    await loadScreen();
  });

  async function loadScreen() {
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
    if (selectedStoreID === '' || !stores.some((store) => store.id === selectedStoreID)) {
      selectedStoreID = stores[0]?.id ?? '';
    }

    await loadMembers();
    loading = false;
  }

  async function loadMembers() {
    members = [];

    if (selectedStoreID === '') {
      return;
    }

    const membersResponse = await fetchStoreMembers(selectedStoreID);
    if (!(await ensureAuthorized(membersResponse.message))) {
      return;
    }

    if (!membersResponse.status || membersResponse.message !== 'SUCCESS') {
      errorMessage = toMessage(membersResponse.message);
      return;
    }

    members = membersResponse.data;
  }

  async function handleStoreChange(event: Event) {
    selectedStoreID = (event.currentTarget as HTMLSelectElement).value;
    errorMessage = '';
    successMessage = '';
    await loadMembers();
  }

  async function submitCreateMember() {
    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await createStoreMember(selectedStoreID, createForm.real_username.trim());
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    createForm = { real_username: '' };
    successMessage = 'Member baru berhasil dibuat dan upstream user code sudah dipetakan.';
    await loadMembers();
  }

  async function ensureAuthorized(message: string) {
    if (message !== 'UNAUTHORIZED') {
      return true;
    }

    await goto('/login');
    return false;
  }

  function currentRole() {
    return $authSession?.user.role ?? '';
  }

  function canCreateMembers() {
    return ['owner', 'dev', 'superadmin'].includes(currentRole());
  }

  function selectedStoreName() {
    return stores.find((store) => store.id === selectedStoreID)?.name ?? '-';
  }

  function toMessage(message: string) {
    switch (message) {
      case 'UNAUTHORIZED':
        return 'Sesi dashboard berakhir. Silakan login ulang.';
      case 'FORBIDDEN':
        return 'Anda tidak punya akses ke member store ini.';
      case 'NOT_FOUND':
        return 'Toko atau member yang diminta tidak ditemukan.';
      case 'INVALID_REAL_USERNAME':
        return 'Username asli member wajib diisi.';
      case 'DUPLICATE_REAL_USERNAME':
        return 'Username member ini sudah ada di toko terpilih.';
      default:
        return 'Terjadi kesalahan. Coba ulangi.';
    }
  }
</script>

<svelte:head>
  <title>Members | onixggr</title>
</svelte:head>

{#if loading}
  <div class="glass-panel rounded-[2rem] p-6">
    <p class="text-sm text-ink-700">Memuat store members dan upstream mapping...</p>
  </div>
{:else}
  <div class="space-y-6">
    <section class="glass-panel rounded-[2rem] p-6">
      <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div class="space-y-2">
          <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
            Store Members
          </p>
          <h1 class="font-display text-3xl font-bold tracking-tight text-ink-900">
            Member toko dan upstream user mapping
          </h1>
          <p class="max-w-3xl text-sm leading-6 text-ink-700">
            Halaman ini menutup Hari 15: username unik per toko, upstream user code 12 karakter,
            dan mapping immutable yang siap dipakai flow game berikutnya.
          </p>
        </div>

        <div class="rounded-[1.5rem] bg-canvas-100 px-4 py-3 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Scope</p>
          <p>Role: {$authSession?.user.role ?? '-'}</p>
          <p>Store terlihat: {stores.length}</p>
          <p>Member di toko aktif: {members.length}</p>
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

    <section class="glass-panel rounded-[2rem] p-6">
      <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
        <label class="space-y-2">
          <span class="text-sm font-medium text-ink-700">Pilih toko</span>
          <select
            bind:value={selectedStoreID}
            class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
            onchange={handleStoreChange}
          >
            {#if stores.length === 0}
              <option value="">Belum ada toko</option>
            {:else}
              {#each stores as store}
                <option value={store.id}>{store.name} ({store.slug})</option>
              {/each}
            {/if}
          </select>
        </label>

        <div class="rounded-[1.5rem] bg-canvas-100 px-4 py-3 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Store aktif</p>
          <p>{selectedStoreName()}</p>
        </div>
      </div>
    </section>

    {#if canCreateMembers() && selectedStoreID !== ''}
      <section class="glass-panel rounded-[2rem] p-6">
        <h2 class="font-display text-2xl font-bold text-ink-900">Buat member baru</h2>
        <p class="mt-2 text-sm leading-6 text-ink-700">
          Username asli hanya unik di dalam toko yang sama. Sistem akan membuat upstream user code
          12 karakter secara otomatis.
        </p>

        <div class="mt-5 grid gap-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
          <label class="space-y-2">
            <span class="text-sm font-medium text-ink-700">Real username</span>
            <input
              bind:value={createForm.real_username}
              class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
              placeholder="member-alpha"
            />
          </label>

          <Button variant="brand" size="lg" onclick={submitCreateMember} disabled={busy}>
            Buat Member
          </Button>
        </div>
      </section>
    {/if}

    <section class="glass-panel rounded-[2rem] p-6">
      <div class="flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
        <div>
          <h2 class="font-display text-2xl font-bold text-ink-900">Daftar member</h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            Mapping ini immutable. Untuk flow game, upstream hanya mengenal `upstream_user_code`.
          </p>
        </div>
      </div>

      {#if selectedStoreID === ''}
        <div class="mt-5 rounded-[1.5rem] bg-canvas-100 px-4 py-4 text-sm text-ink-700">
          Belum ada toko yang bisa dipilih.
        </div>
      {:else if members.length === 0}
        <div class="mt-5 rounded-[1.5rem] bg-canvas-100 px-4 py-4 text-sm text-ink-700">
          Belum ada member untuk toko ini.
        </div>
      {:else}
        <div class="mt-5 overflow-x-auto">
          <table class="min-w-full border-separate border-spacing-y-3">
            <thead>
              <tr class="text-left text-xs uppercase tracking-[0.18em] text-ink-500">
                <th class="px-3">Real Username</th>
                <th class="px-3">Upstream Code</th>
                <th class="px-3">Status</th>
                <th class="px-3">Created</th>
              </tr>
            </thead>
            <tbody>
              {#each members as member}
                <tr class="rounded-[1.5rem] bg-canvas-100 text-sm text-ink-800">
                  <td class="rounded-l-[1.5rem] px-3 py-4 font-medium text-ink-900">
                    {member.real_username}
                  </td>
                  <td class="px-3 py-4 font-mono text-xs tracking-[0.2em]">
                    {member.upstream_user_code}
                  </td>
                  <td class="px-3 py-4 uppercase tracking-[0.12em] text-ink-600">
                    {member.status}
                  </td>
                  <td class="rounded-r-[1.5rem] px-3 py-4 text-ink-600">
                    {new Date(member.created_at).toLocaleString('id-ID')}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>
  </div>
{/if}
