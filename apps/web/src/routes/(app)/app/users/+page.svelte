<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import DateRangeFilter from '$lib/components/app/date-range-filter.svelte';
  import EmptyState from '$lib/components/app/empty-state.svelte';
  import ExportActions from '$lib/components/app/export-actions.svelte';
  import MetricCard from '$lib/components/app/metric-card.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import PaginationControls from '$lib/components/app/pagination-controls.svelte';
  import Button from '$lib/components/ui/button/button.svelte';
  import { authSession, initializeAuthSession } from '$lib/auth/client';
  import { exportRowsToCSV, exportRowsToPDF, exportRowsToXLSX } from '$lib/exporters';
  import { formatDateTime, formatNumber } from '$lib/formatters';
  import {
    createManagedUser,
    fetchUserDirectory,
    updateManagedUserStatus,
    type ManagedUser,
    type ManagedUserRole,
    type UserDirectorySummary,
  } from '$lib/users/client';

  const emptySummary: UserDirectorySummary = {
    total_count: 0,
    owner_count: 0,
    superadmin_count: 0,
    dev_count: 0,
    active_count: 0,
    inactive_count: 0,
  };

  let initialized = false;
  let loading = true;
  let busy = false;
  let errorMessage = '';
  let successMessage = '';

  let users: ManagedUser[] = [];
  let summary: UserDirectorySummary = { ...emptySummary };
  let totalCount = 0;

  let searchTerm = '';
  let roleFilter: 'all' | ManagedUserRole = 'all';
  let activeFilter: 'all' | 'true' | 'false' = 'all';
  let createdFrom = '';
  let createdTo = '';
  let page = 1;
  let pageSize = 8;
  let lastPageKey = '';

  let createForm: {
    email: string;
    username: string;
    password: string;
    role: 'owner' | 'superadmin';
  } = {
    email: '',
    username: '',
    password: '',
    role: 'owner',
  };

  $: currentRole = $authSession?.user.role ?? '';
  $: canViewUsers = currentRole === 'dev' || currentRole === 'superadmin';
  $: availableRoles = currentRole === 'dev' ? ['owner', 'superadmin'] : ['owner'];
  $: if (!availableRoles.includes(createForm.role)) {
    createForm = { ...createForm, role: availableRoles[0] as 'owner' | 'superadmin' };
  }

  onMount(async () => {
    await initializeAuthSession();

    if (!$authSession) {
      await goto('/login');
      return;
    }

    initialized = true;
    await loadDirectory();
  });

  $: if (initialized && canViewUsers) {
    const nextPageKey = `${page}:${pageSize}`;
    if (nextPageKey !== lastPageKey) {
      lastPageKey = nextPageKey;
      void loadDirectory();
    }
  }

  async function loadDirectory() {
    if (!canViewUsers) {
      loading = false;
      users = [];
      summary = { ...emptySummary };
      totalCount = 0;
      return;
    }

    loading = true;
    errorMessage = '';

    const response = await fetchUserDirectory({
      query: searchTerm,
      role: roleFilter,
      isActive: activeFilter,
      createdFrom,
      createdTo,
      limit: pageSize,
      offset: (page - 1) * pageSize,
    });
    loading = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    users = response.data.items ?? [];
    summary = response.data.summary ?? { ...emptySummary };
    totalCount = summary.total_count ?? 0;
  }

  async function submitCreateUser() {
    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await createManagedUser({
      email: createForm.email.trim(),
      username: createForm.username.trim(),
      password: createForm.password,
      role: createForm.role,
    });
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    successMessage =
      response.data.role === 'owner'
        ? 'Owner berhasil dibuat. Langkah berikutnya: login sebagai owner lalu buat store pertama.'
        : 'Superadmin berhasil dibuat.';
    createForm = {
      email: '',
      username: '',
      password: '',
      role: availableRoles[0] as 'owner' | 'superadmin',
    };
    page = 1;
    lastPageKey = '';
    await loadDirectory();
  }

  async function toggleUser(user: ManagedUser) {
    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await updateManagedUserStatus(user.id, !user.is_active);
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    successMessage = response.data.is_active
      ? `Akun ${response.data.username} diaktifkan kembali.`
      : `Akun ${response.data.username} dinonaktifkan.`;
    await loadDirectory();
  }

  async function applyFilters() {
    page = 1;
    lastPageKey = '';
    await loadDirectory();
  }

  async function resetFilters() {
    searchTerm = '';
    roleFilter = 'all';
    activeFilter = 'all';
    createdFrom = '';
    createdTo = '';
    page = 1;
    lastPageKey = '';
    await loadDirectory();
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
      case 'UNAUTHORIZED':
        return 'Sesi dashboard berakhir. Login ulang diperlukan.';
      case 'FORBIDDEN':
        return 'Surface ini hanya tersedia untuk dev atau superadmin.';
      case 'INVALID_INPUT':
        return 'Email, username, password, dan role wajib valid.';
      case 'INVALID_ROLE':
        return 'Role yang dipilih tidak valid untuk provisioning ini.';
      case 'ROLE_PROVISION_FORBIDDEN':
        return 'Role itu tidak boleh diprovisi oleh akun Anda.';
      case 'DUPLICATE_IDENTITY':
        return 'Email atau username sudah dipakai.';
      case 'STATUS_UPDATE_FORBIDDEN':
        return 'Akun target tidak boleh diubah dari surface ini.';
      case 'CANNOT_DEACTIVATE_SELF':
        return 'Akun sendiri tidak boleh dinonaktifkan dari sesi aktif.';
      case 'NOT_FOUND':
        return 'User target tidak ditemukan.';
      default:
        return 'Terjadi kesalahan. Coba ulangi beberapa saat lagi.';
    }
  }

  function exportUsersToCSV() {
    exportRowsToCSV(
      'managed-users-page',
      [
        { label: 'Username', value: (user) => user.username },
        { label: 'Email', value: (user) => user.email },
        { label: 'Role', value: (user) => user.role },
        { label: 'Status', value: (user) => (user.is_active ? 'active' : 'inactive') },
        { label: 'Created At', value: (user) => formatDateTime(user.created_at) },
        { label: 'Last Login', value: (user) => formatDateTime(user.last_login_at) },
      ],
      users,
    );
  }

  function exportUsersToXLSX() {
    return exportRowsToXLSX(
      'managed-users-page',
      'Managed Users',
      [
        { label: 'Username', value: (user) => user.username },
        { label: 'Email', value: (user) => user.email },
        { label: 'Role', value: (user) => user.role },
        { label: 'Status', value: (user) => (user.is_active ? 'active' : 'inactive') },
        { label: 'Created At', value: (user) => formatDateTime(user.created_at) },
        { label: 'Last Login', value: (user) => formatDateTime(user.last_login_at) },
      ],
      users,
    );
  }

  function exportUsersToPDF() {
    return exportRowsToPDF(
      'managed-users-page',
      'Managed Users (Current Page)',
      [
        { label: 'Username', value: (user) => user.username },
        { label: 'Email', value: (user) => user.email },
        { label: 'Role', value: (user) => user.role },
        { label: 'Status', value: (user) => (user.is_active ? 'active' : 'inactive') },
      ],
      users,
    );
  }

  function nextStep(user: ManagedUser) {
    if (user.role === 'owner') {
      return 'Owner login lalu buka Stores untuk membuat toko pertama, callback URL, token, dan staff.';
    }

    if (user.role === 'superadmin') {
      return 'Superadmin mendapat visibility platform, audit, notifications, ops, dan observability.';
    }

    return 'Akun dev diproteksi dan tidak diprovisi dari dashboard ini.';
  }

  function canToggle(user: ManagedUser) {
    if (currentRole === 'dev') {
      return user.role !== 'dev';
    }

    return user.role === 'owner';
  }
</script>

<svelte:head>
  <title>Users | onixggr</title>
</svelte:head>

{#if loading}
  <div class="glass-panel rounded-[2.4rem] p-6">
    <p class="text-sm text-ink-700">Memuat user directory, provisioning role, dan onboarding owner...</p>
  </div>
{:else if !canViewUsers}
  <EmptyState
    title="Surface ini hanya untuk role platform"
    body="Provision owner dan superadmin hanya tersedia untuk dev atau superadmin. Owner tetap membuat karyawan dari halaman Stores."
    actionLabel="Kembali ke dashboard"
    actionHref="/app"
  />
{:else}
  <div class="space-y-6">
    <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
      <div class="grid gap-6 2xl:grid-cols-[1.08fr_0.92fr]">
        <div class="space-y-4">
          <div class="flex flex-wrap gap-3">
            <span class="status-chip">user onboarding</span>
            <span class="status-chip">{currentRole}</span>
            <span class="status-chip">{formatNumber(summary.total_count)} identity</span>
          </div>
          <div class="space-y-3">
            <p class="section-kicker">Provision owners</p>
            <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
              Jalur resmi untuk mendaftarkan owner sebelum mereka membuat store.
            </h1>
            <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
              Blueprint menuntut internal user management untuk role platform. Halaman ini sekarang
              menjadi pintu masuk provisioning owner, reaktivasi akun, dan kontrol onboarding
              tenant tanpa harus menyentuh database manual.
            </p>
          </div>
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <MetricCard
            class="h-full"
            eyebrow="Owners"
            title="Owner accounts"
            value={formatNumber(summary.owner_count)}
            detail="Owner inilah yang nantinya login lalu membuat store dan token integrasi."
            tone="brand"
          />
          <MetricCard
            class="h-full"
            eyebrow="Platform"
            title="Superadmins"
            value={formatNumber(summary.superadmin_count)}
            detail="Role platform dengan monitoring lintas store dan surface ops."
          />
        </div>
      </div>
    </section>

    {#if errorMessage !== ''}
      <Notice tone="error" title="User Action Error" message={errorMessage} />
    {/if}

    {#if successMessage !== ''}
      <Notice tone="success" title="User Action Success" message={successMessage} />
    {/if}

    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <MetricCard
        eyebrow="Directory"
        title="Matched identities"
        value={formatNumber(summary.total_count)}
        detail="Total user platform + owner yang cocok dengan filter aktif."
        tone="brand"
      />
      <MetricCard
        eyebrow="Active"
        title="Active accounts"
        value={formatNumber(summary.active_count)}
        detail="Akun aktif yang masih bisa login ke dashboard."
      />
      <MetricCard
        eyebrow="Inactive"
        title="Disabled accounts"
        value={formatNumber(summary.inactive_count)}
        detail="Akun yang sengaja dimatikan tanpa menghapus histori audit."
        tone={summary.inactive_count > 0 ? 'accent' : 'default'}
      />
      <MetricCard
        eyebrow="Dev"
        title="Core operators"
        value={formatNumber(summary.dev_count)}
        detail="Akun dev tetap diproteksi dan tidak bisa diprovisi dari UI ini."
      />
    </div>

    <div class="grid gap-6 2xl:grid-cols-[0.94fr_1.06fr]">
      <section class="form-surface">
        <div class="space-y-3">
          <p class="section-kicker !text-brand-700">Create dashboard user</p>
          <h2 class="font-display text-3xl font-bold tracking-tight text-ink-900">
            Provision owner atau superadmin
          </h2>
          <p class="text-sm leading-7 text-ink-700">
            Flow yang paling sehat sekarang: dev mendaftarkan owner di sini, owner login, lalu owner
            membuat toko di halaman Stores. Ini menjaga ownership tetap sesuai blueprint.
          </p>
        </div>

        <div class="form-grid mt-6">
          <label class="field-stack">
            <span class="field-label">Email</span>
            <input bind:value={createForm.email} class="field-input" placeholder="owner@example.com" />
          </label>

          <label class="field-stack">
            <span class="field-label">Username</span>
            <input bind:value={createForm.username} class="field-input" placeholder="owner-alpha" />
          </label>

          <label class="field-stack">
            <span class="field-label">Password awal</span>
            <input bind:value={createForm.password} class="field-input" type="password" placeholder="OwnerDemo123!" />
          </label>

          <label class="field-stack">
            <span class="field-label">Role</span>
            <select bind:value={createForm.role} class="field-select">
              {#each availableRoles as roleOption}
                <option value={roleOption}>{roleOption}</option>
              {/each}
            </select>
          </label>
        </div>

        <div class="stack-actions mt-5">
          <Button variant="brand" size="lg" onclick={submitCreateUser} disabled={busy}>
            Provision User
          </Button>
          <a class="surface-chip" href="/app/stores">Owner membuat store di Stores</a>
          <a class="surface-chip" href="/app/api-docs">Owner integrasi di API Docs</a>
        </div>
      </section>

      <section class="glass-panel rounded-[2rem] p-6">
        <div class="space-y-3">
          <p class="section-kicker !text-brand-700">Onboarding playbook</p>
          <h2 class="font-display text-3xl font-bold tracking-tight text-ink-900">
            Urutan yang benar
          </h2>
        </div>

        <ol class="command-list mt-5">
          <li>Dev atau superadmin membuat akun owner dari halaman ini.</li>
          <li>Owner login ke dashboard, lalu buka halaman Stores untuk membuat toko pertama.</li>
          <li>Store yang baru dibuat langsung mengeluarkan one-time API token.</li>
          <li>Owner set callback URL, bank account, members, lalu buka API Docs untuk integrasi website.</li>
        </ol>

        <div class="mt-5 grid gap-3 sm:grid-cols-2">
          <div class="rounded-[1.5rem] bg-canvas-50 px-4 py-4">
            <p class="text-sm font-semibold text-ink-900">Why not manual DB?</p>
            <p class="mt-2 text-sm leading-6 text-ink-700">
              Karena onboarding harus tercatat di audit log dan tetap menjaga boundary role sesuai
              blueprint.
            </p>
          </div>
          <div class="rounded-[1.5rem] bg-canvas-50 px-4 py-4">
            <p class="text-sm font-semibold text-ink-900">Why owner creates store?</p>
            <p class="mt-2 text-sm leading-6 text-ink-700">
              Karena token awal diterbitkan saat create store, dan ownership token harus muncul di
              sesi owner.
            </p>
          </div>
        </div>
      </section>
    </div>

    <section class="toolbar-panel">
      <div class="toolbar-grid">
        <label class="field-stack">
          <span class="field-label">Search users</span>
          <input bind:value={searchTerm} class="field-input" type="search" placeholder="Cari email atau username" />
        </label>

        <label class="field-stack">
          <span class="field-label">Role filter</span>
          <select bind:value={roleFilter} class="field-select">
            <option value="all">All managed roles</option>
            <option value="owner">owner</option>
            {#if currentRole === 'dev'}
              <option value="superadmin">superadmin</option>
              <option value="dev">dev</option>
            {/if}
          </select>
        </label>

        <label class="field-stack">
          <span class="field-label">Status filter</span>
          <select bind:value={activeFilter} class="field-select">
            <option value="all">All statuses</option>
            <option value="true">active</option>
            <option value="false">inactive</option>
          </select>
        </label>
      </div>

      <div class="toolbar-actions mt-4">
        <Button variant="brand" onclick={applyFilters}>Apply Filters</Button>
        <Button variant="outline" onclick={resetFilters}>Reset</Button>
      </div>
    </section>

    <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(18rem,24rem)]">
      <DateRangeFilter bind:start={createdFrom} bind:end={createdTo} label="Created at" />
      <ExportActions
        count={users.length}
        disabled={users.length === 0}
        onCsv={exportUsersToCSV}
        onXlsx={exportUsersToXLSX}
        onPdf={exportUsersToPDF}
      />
    </div>

    {#if users.length === 0}
      <EmptyState
        title="Belum ada user terfilter"
        body="Tidak ada owner atau role platform lain yang cocok dengan filter saat ini."
      />
    {:else}
      <div class="grid gap-4 xl:grid-cols-2">
        {#each users as user}
          <article class="glass-panel rounded-[2rem] p-5">
            <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div class="space-y-3">
                <div class="flex flex-wrap gap-2">
                  <span class="surface-chip">{user.role}</span>
                  <span class="surface-chip">{user.is_active ? 'active' : 'inactive'}</span>
                </div>
                <div>
                  <p class="font-display text-2xl font-bold tracking-tight text-ink-900">
                    {user.username}
                  </p>
                  <p class="mt-1 text-sm text-ink-600">{user.email}</p>
                </div>
              </div>

              {#if canToggle(user)}
                <Button variant="outline" size="sm" onclick={() => toggleUser(user)} disabled={busy}>
                  {user.is_active ? 'Deactivate' : 'Reactivate'}
                </Button>
              {/if}
            </div>

            <div class="mt-5 grid gap-3 sm:grid-cols-2">
              <div class="rounded-[1.35rem] bg-canvas-50 px-4 py-4">
                <p class="text-[0.7rem] font-semibold uppercase tracking-[0.24em] text-ink-500">Created</p>
                <p class="mt-2 text-sm font-semibold text-ink-900">{formatDateTime(user.created_at)}</p>
              </div>
              <div class="rounded-[1.35rem] bg-canvas-50 px-4 py-4">
                <p class="text-[0.7rem] font-semibold uppercase tracking-[0.24em] text-ink-500">Last login</p>
                <p class="mt-2 text-sm font-semibold text-ink-900">{formatDateTime(user.last_login_at)}</p>
              </div>
            </div>

            <div class="mt-5 rounded-[1.5rem] border border-ink-100 bg-white/72 px-4 py-4">
              <p class="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-ink-500">
                Next step
              </p>
              <p class="mt-2 text-sm leading-6 text-ink-700">{nextStep(user)}</p>
            </div>
          </article>
        {/each}
      </div>
    {/if}

    <PaginationControls bind:page bind:pageSize totalItems={totalCount} />
  </div>
{/if}
