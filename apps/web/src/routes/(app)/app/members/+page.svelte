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
  import { createStoreMember, fetchStoreMembers, type StoreMember } from '$lib/store-members/client';
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
  let members: StoreMember[] = [];
  let totalMemberCount = 0;
  let activeMemberCount = 0;
  let inactiveMemberCount = 0;
  let selectedStoreID = '';
  let selectedStore: Store | null = null;
  let searchTerm = '';
  let statusFilter: StoreMember['status'] | 'all' = 'all';
  let createdFrom = '';
  let createdTo = '';
  let memberPage = 1;
  let memberPageSize = 6;
  let lastQueryKey = '';
  let createForm = {
    real_username: ''
  };

  $: createRealUsername = createForm.real_username.trim();
  $: if (!loading && selectedStoreID !== '') {
    const nextKey = `${selectedStoreID}:${memberPage}:${memberPageSize}`;
    if (nextKey !== lastQueryKey) {
      void loadMembers();
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
        await loadMembers();
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

  async function loadMembers() {
    members = [];
    totalMemberCount = 0;
    activeMemberCount = 0;
    inactiveMemberCount = 0;

    if (selectedStoreID === '') {
      lastQueryKey = '';
      return;
    }

    const membersResponse = await fetchStoreMembers(selectedStoreID, {
      query: searchTerm || undefined,
      status: statusFilter === 'all' ? undefined : statusFilter,
      limit: memberPageSize,
      offset: (memberPage - 1) * memberPageSize,
      createdFrom: createdFrom || undefined,
      createdTo: createdTo || undefined
    });
    if (!(await ensureAuthorized(membersResponse.message))) {
      return;
    }

    if (!membersResponse.status || membersResponse.message !== 'SUCCESS') {
      errorMessage = toMessage(membersResponse.message);
      return;
    }

    members = membersResponse.data.items ?? [];
    totalMemberCount = membersResponse.data.summary?.total_count ?? 0;
    activeMemberCount = membersResponse.data.summary?.active_count ?? 0;
    inactiveMemberCount = membersResponse.data.summary?.inactive_count ?? 0;
    lastQueryKey = `${selectedStoreID}:${memberPage}:${memberPageSize}`;
  }

  async function handleStoreScopeChange(event: CustomEvent<{ storeID: string; store: Store | null }>) {
    selectedStoreID = event.detail.storeID;
    selectedStore = event.detail.store;
    setPreferredStoreID(selectedStoreID);
    errorMessage = '';
    successMessage = '';
    memberPage = 1;
    await loadMembers();
  }

  async function submitCreateMember() {
    if (selectedStoreID === '') {
      errorMessage = 'Pilih toko terlebih dahulu sebelum membuat member baru.';
      return;
    }

    if (createRealUsername === '') {
      errorMessage = 'Real username wajib diisi sebelum submit.';
      return;
    }

    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await createStoreMember(selectedStoreID, createRealUsername);
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
    return selectedStore?.name ?? '-';
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

  function exportMembersToCSV() {
    exportRowsToCSV(
      `${selectedStoreID || 'store'}-members`,
      [
        { label: 'Real Username', value: (member) => member.real_username },
        { label: 'Upstream User Code', value: (member) => member.upstream_user_code },
        { label: 'Status', value: (member) => member.status },
        { label: 'Created At', value: (member) => formatDateTime(member.created_at) },
        { label: 'Updated At', value: (member) => formatDateTime(member.updated_at) }
      ],
      members,
    );
  }

  function exportMembersToXLSX() {
    exportRowsToXLSX(
      `${selectedStoreID || 'store'}-members`,
      'Members',
      [
        { label: 'Real Username', value: (member) => member.real_username },
        { label: 'Upstream User Code', value: (member) => member.upstream_user_code },
        { label: 'Status', value: (member) => member.status },
        { label: 'Created At', value: (member) => formatDateTime(member.created_at) },
        { label: 'Updated At', value: (member) => formatDateTime(member.updated_at) }
      ],
      members,
    );
  }

  function exportMembersToPDF() {
    exportRowsToPDF(
      `${selectedStoreID || 'store'}-members`,
      'Store Members',
      [
        { label: 'Real Username', value: (member) => member.real_username },
        { label: 'Upstream Code', value: (member) => member.upstream_user_code },
        { label: 'Status', value: (member) => member.status },
        { label: 'Created', value: (member) => formatDateTime(member.created_at) }
      ],
      members,
    );
  }

  async function applyFilters() {
    memberPage = 1;
    await loadMembers();
  }

  async function resetFilters() {
    searchTerm = '';
    statusFilter = 'all';
    createdFrom = '';
    createdTo = '';
    memberPage = 1;
    await loadMembers();
  }
</script>

<svelte:head>
  <title>Members | onixggr</title>
</svelte:head>

{#if loading}
  <PageSkeleton blocks={4} />
{:else}
  <div class="space-y-6">
    <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
      <div class="flex flex-col gap-5 md:flex-row md:items-start md:justify-between">
        <div class="space-y-2">
          <p class="section-kicker">
            Store Members
          </p>
          <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
            Identity desk untuk member toko dan mapping upstream.
          </h1>
          <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
            Halaman ini menutup Hari 15: username unik per toko, upstream user code 12 karakter,
            dan mapping immutable yang siap dipakai flow game berikutnya.
          </p>
        </div>

        <div class="rounded-[1.8rem] border border-white/12 bg-white/7 px-4 py-4 text-sm text-white/72 backdrop-blur">
          <p class="font-semibold text-white">Scope</p>
          <p class="mt-2">Role: {$authSession?.user.role ?? '-'}</p>
          <p>Store terlihat: {formatNumber(storeScopeTotalCount)}</p>
          <p>Member di toko aktif: {formatNumber(totalMemberCount)}</p>
        </div>
      </div>
    </section>

    {#if errorMessage}
      <Notice tone="error" title="Permintaan belum bisa diproses" message={errorMessage} />
    {/if}

    {#if successMessage}
      <Notice tone="success" title="Perubahan tersimpan" message={successMessage} />
    {/if}

    {#if storeScopeLoading}
      <PageSkeleton blocks={2} />
    {:else if storeScopeTotalCount === 0}
      <EmptyState
        eyebrow="Store Scope"
        title="Belum ada toko di sesi ini"
        body="Member mapping baru bisa dibuat setelah ada toko yang masuk scope owner, dev, superadmin, atau assignment karyawan."
        actionHref="/app/stores"
        actionLabel="Buka Stores"
      />
    {:else}
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          eyebrow="Store"
          title="Visible stores"
          value={formatNumber(storeScopeTotalCount)}
          detail="Jumlah toko yang bisa dipilih dari sesi ini."
          tone="brand"
        />
        <MetricCard
          eyebrow="Current Store"
          title="Mapped members"
          value={formatNumber(totalMemberCount)}
          detail="Semua identity mapping terfilter di store aktif."
        />
        <MetricCard
          eyebrow="Status"
          title="Active members"
          value={formatNumber(activeMemberCount)}
          detail="Member aktif yang siap dipakai flow game dan QRIS member payment."
          tone="brand"
        />
        <MetricCard
          eyebrow="Status"
          title="Inactive members"
          value={formatNumber(inactiveMemberCount)}
          detail="Member tidak aktif pada store aktif."
          tone={inactiveMemberCount > 0 ? 'accent' : 'default'}
        />
      </div>

      <StoreScopePicker
        bind:selectedStoreID
        bind:selectedStore
        bind:loading={storeScopeLoading}
        bind:totalCount={storeScopeTotalCount}
        compact
        title="Store scope untuk member directory"
        description="Picker ini memakai directory endpoint backend dan tetap ringan walau jumlah toko membesar."
        placeholder="Cari store atau slug untuk mapping member"
        on:change={handleStoreScopeChange}
      />

      {#if canCreateMembers() && selectedStoreID !== ''}
        <section class="glass-panel rounded-4xl p-6">
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
              <p class="text-xs leading-5 text-ink-500">
                Real username akan dipakai owner sebagai identitas utama member.
              </p>
            </label>

            <Button variant="brand" size="lg" onclick={submitCreateMember} disabled={busy || createRealUsername === ''}>
              Buat Member
            </Button>
          </div>
        </section>
      {/if}

      <section class="glass-panel rounded-4xl p-6">
        <div class="flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
          <div>
            <h2 class="font-display text-2xl font-bold text-ink-900">Daftar member</h2>
            <p class="mt-2 text-sm leading-6 text-ink-700">
              Mapping ini immutable. Query, status, rentang waktu, dan pagination sekarang
              dieksekusi di backend agar tetap ringan saat row member sangat besar.
            </p>
          </div>
        </div>

        <div class="mt-5 grid gap-4 2xl:grid-cols-[12rem_minmax(0,1fr)]">
          <label class="space-y-2">
            <span class="text-sm font-medium text-ink-700">Status</span>
            <select
              bind:value={statusFilter}
              class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
            >
              <option value="all">Semua status</option>
              <option value="active">Active</option>
              <option value="inactive">Inactive</option>
            </select>
          </label>

          <label class="space-y-2">
            <span class="text-sm font-medium text-ink-700">Cari member</span>
            <input
              bind:value={searchTerm}
              class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
              placeholder="Cari username atau upstream code"
            />
          </label>
        </div>

        <div class="mt-4 grid gap-4 2xl:grid-cols-[minmax(0,1fr)_minmax(18rem,24rem)]">
          <DateRangeFilter bind:start={createdFrom} bind:end={createdTo} label="Created at" />
          <ExportActions
            count={members.length}
            disabled={members.length === 0}
            onCsv={exportMembersToCSV}
            onXlsx={exportMembersToXLSX}
            onPdf={exportMembersToPDF}
          />
        </div>

        <div class="mt-4 flex flex-wrap gap-3">
          <Button variant="brand" size="lg" onclick={applyFilters}>
            Apply Filters
          </Button>
          <Button variant="outline" size="lg" onclick={resetFilters}>
            Reset
          </Button>
        </div>

      {#if selectedStoreID === ''}
        <div class="mt-5">
          <EmptyState
            eyebrow="Store Switch"
            title="Pilih toko lebih dulu"
            body="Daftar member akan muncul setelah Anda memilih toko aktif dari store switch di halaman ini atau di app shell."
          />
        </div>
      {:else if totalMemberCount === 0}
        <div class="mt-5">
          <EmptyState
            eyebrow="Member List"
            title="Belum ada member di toko ini"
            body="Belum ada mapping member untuk store terpilih. Buat member pertama agar flow game dan QRIS member payment punya target username yang valid."
          />
        </div>
      {:else if members.length === 0}
        <div class="mt-5">
          <EmptyState
            eyebrow="Filter Result"
            title="Tidak ada member yang cocok"
            body={`Tidak ada hasil untuk filter "${searchTerm}" dan rentang waktu aktif. Coba kata kunci lain atau reset filter.`}
          />
        </div>
      {:else}
        <div class="mt-5 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {#each members as member}
            <article class="rounded-[1.7rem] border border-ink-100 bg-white p-4 shadow-[0_16px_34px_rgba(7,16,12,0.08)]">
              <div class="flex items-start justify-between gap-3">
                <div>
                  <p class="text-sm font-semibold text-ink-900">{member.real_username}</p>
                  <p class="mt-2 text-[0.72rem] font-semibold uppercase tracking-[0.22em] text-brand-700">
                    {member.status}
                  </p>
                </div>
                <span class="surface-chip">{member.status}</span>
              </div>

              <div class="mt-4 rounded-[1.4rem] bg-canvas-50 px-4 py-4">
                <p class="text-[0.72rem] font-semibold uppercase tracking-[0.22em] text-ink-300">
                  Upstream code
                </p>
                <p class="mt-2 break-all font-mono text-xs tracking-[0.22em] text-ink-900">
                  {member.upstream_user_code}
                </p>
              </div>

              <p class="mt-4 text-xs leading-5 text-ink-500">
                Created {formatDateTime(member.created_at)}
              </p>
            </article>
          {/each}
        </div>

        <div class="mt-5">
          <PaginationControls bind:page={memberPage} bind:pageSize={memberPageSize} totalItems={totalMemberCount} />
        </div>
      {/if}
      </section>
    {/if}
  </div>
{/if}
