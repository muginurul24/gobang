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
  import { formatCurrency, formatDateTime, formatNumber } from '$lib/formatters';
  import {
    assignStoreStaff,
    createEmployee,
    createStore,
    deleteStore,
    fetchEmployeeDirectory,
    fetchStoreDirectory,
    fetchStoreStaffDirectory,
    isStoreLowBalance,
    rotateStoreToken,
    type StaffUser,
    type Store,
    type StoreDirectorySummary,
    unassignStoreStaff,
    updateStore,
    updateStoreCallbackURL,
  } from '$lib/stores/client';

  type StoreFormState = {
    name: string;
    status: string;
    low_balance_threshold: string;
    callback_url: string;
    assign_user_id: string;
  };

  const emptyStoreSummary: StoreDirectorySummary = {
    total_count: 0,
    active_count: 0,
    inactive_count: 0,
    banned_count: 0,
    deleted_count: 0,
    low_balance_count: 0,
  };

  let initialized = false;
  let loading = true;
  let directoryLoading = false;
  let employeeDirectoryLoading = false;
  let busy = false;
  let errorMessage = '';
  let successMessage = '';

  let stores: Store[] = [];
  let storeSummary: StoreDirectorySummary = { ...emptyStoreSummary };
  let storeTotalCount = 0;
  let employees: StaffUser[] = [];
  let employeeTotalCount = 0;

  let staffByStore: Record<string, StaffUser[]> = {};
  let staffCountByStore: Record<string, number> = {};
  let storeForms: Record<string, StoreFormState> = {};
  let revealedTokens: Record<string, string> = {};

  let createStoreForm = {
    name: '',
    slug: '',
    low_balance_threshold: '',
  };
  let createEmployeeForm = {
    email: '',
    username: '',
    password: '',
  };

  let storeSearchTerm = '';
  let storeStatusFilter: Store['status'] | 'all' = 'all';
  let lowBalanceFilter: 'all' | 'low_balance' | 'healthy' = 'all';
  let storeCreatedFrom = '';
  let storeCreatedTo = '';
  let storePage = 1;
  let storePageSize = 4;
  let lastStorePageKey = '';

  let employeeSearchTerm = '';
  let employeeCreatedFrom = '';
  let employeeCreatedTo = '';
  let employeePage = 1;
  let employeePageSize = 6;
  let lastEmployeePageKey = '';

  let staffSearchTerm = '';
  let staffAssignedFrom = '';
  let staffAssignedTo = '';

  $: role = $authSession?.user.role ?? '';
  $: visibleEmployeeOptions = employees;
  $: revealedTokenCount = Object.keys(revealedTokens).length;
  $: visibleStoreCount = stores.length;
  $: visibleStaffCount = Object.values(staffByStore).reduce((total, users) => total + users.length, 0);
  $: totalAssignedStaffCount = Object.values(staffCountByStore).reduce((total, count) => total + count, 0);

  onMount(async () => {
    await initializeAuthSession();

    if (!$authSession) {
      await goto('/login');
      return;
    }

    initialized = true;
    await loadScreen();
  });

  $: if (initialized) {
    const nextStorePageKey = `${storePage}:${storePageSize}`;
    if (nextStorePageKey !== lastStorePageKey) {
      lastStorePageKey = nextStorePageKey;
      void loadStoreDirectory();
    }
  }

  $: if (initialized && canManageEmployees()) {
    const nextEmployeePageKey = `${employeePage}:${employeePageSize}`;
    if (nextEmployeePageKey !== lastEmployeePageKey) {
      lastEmployeePageKey = nextEmployeePageKey;
      void loadEmployeeDirectory();
    }
  }

  async function loadScreen() {
    loading = true;
    errorMessage = '';

    await loadStoreDirectory();

    if (canManageEmployees()) {
      await loadEmployeeDirectory();
    } else {
      employees = [];
      employeeTotalCount = 0;
    }

    loading = false;
  }

  async function loadStoreDirectory() {
    directoryLoading = true;
    errorMessage = '';

    const response = await fetchStoreDirectory({
      query: storeSearchTerm,
      status: storeStatusFilter,
      lowBalanceState: lowBalanceFilter,
      createdFrom: storeCreatedFrom,
      createdTo: storeCreatedTo,
      limit: storePageSize,
      offset: (storePage - 1) * storePageSize,
    });
    directoryLoading = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    stores = response.data.items ?? [];
    storeSummary = response.data.summary ?? { ...emptyStoreSummary };
    storeTotalCount = storeSummary.total_count ?? 0;
    syncStoreForms(stores);
    await loadVisibleStoreStaff();
  }

  async function loadEmployeeDirectory() {
    if (!canManageEmployees()) {
      employees = [];
      employeeTotalCount = 0;
      return;
    }

    employeeDirectoryLoading = true;
    errorMessage = '';

    const response = await fetchEmployeeDirectory({
      query: employeeSearchTerm,
      createdFrom: employeeCreatedFrom,
      createdTo: employeeCreatedTo,
      limit: employeePageSize,
      offset: (employeePage - 1) * employeePageSize,
    });
    employeeDirectoryLoading = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    employees = response.data.items ?? [];
    employeeTotalCount = response.data.total_count ?? 0;
  }

  async function loadVisibleStoreStaff() {
    if (!canViewStaffLists() || stores.length === 0) {
      staffByStore = {};
      staffCountByStore = {};
      return;
    }

    const entries = await Promise.all(
      stores.map(async (store) => {
        const response = await fetchStoreStaffDirectory(store.id, {
          query: staffSearchTerm,
          assignedFrom: staffAssignedFrom,
          assignedTo: staffAssignedTo,
          limit: 6,
          offset: 0,
        });
        return [store.id, response] as const;
      }),
    );

    const nextStaff: Record<string, StaffUser[]> = {};
    const nextCounts: Record<string, number> = {};

    for (const [storeID, response] of entries) {
      if (response.status && response.message === 'SUCCESS') {
        nextStaff[storeID] = response.data.items ?? [];
        nextCounts[storeID] = response.data.total_count ?? 0;
      } else {
        nextStaff[storeID] = [];
        nextCounts[storeID] = 0;
      }
    }

    staffByStore = nextStaff;
    staffCountByStore = nextCounts;
  }

  function syncStoreForms(items: Store[]) {
    const nextForms: Record<string, StoreFormState> = {};
    for (const store of items) {
      nextForms[store.id] = {
        name: store.name,
        status: store.status,
        low_balance_threshold: store.low_balance_threshold ?? '',
        callback_url: store.callback_url ?? '',
        assign_user_id: storeForms[store.id]?.assign_user_id ?? '',
      };
    }

    storeForms = nextForms;
  }

  async function submitCreateStore() {
    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await createStore({
      name: createStoreForm.name.trim(),
      slug: createStoreForm.slug.trim(),
      low_balance_threshold: normalizeOptional(createStoreForm.low_balance_threshold),
    });
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }
    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    createStoreForm = { name: '', slug: '', low_balance_threshold: '' };
    if (response.data.api_token) {
      revealedTokens = {
        ...revealedTokens,
        [response.data.id]: response.data.api_token,
      };
    }

    successMessage = 'Toko baru dibuat. Simpan token integrasi yang baru muncul.';
    storePage = 1;
    lastStorePageKey = '';
    await loadStoreDirectory();
  }

  async function submitStoreUpdate(storeID: string) {
    const form = storeForms[storeID];
    if (!form) {
      return;
    }

    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await updateStore(storeID, {
      name: form.name.trim(),
      status: form.status,
      low_balance_threshold: normalizeOptional(form.low_balance_threshold),
    });
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }
    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    successMessage = 'Pengaturan toko diperbarui.';
    await loadStoreDirectory();
  }

  async function submitCallbackUpdate(storeID: string) {
    const form = storeForms[storeID];
    if (!form) {
      return;
    }

    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await updateStoreCallbackURL(storeID, form.callback_url.trim());
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }
    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    successMessage = 'Callback URL tersimpan.';
    await loadStoreDirectory();
  }

  async function submitRotateToken(storeID: string) {
    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await rotateStoreToken(storeID);
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }
    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    revealedTokens = {
      ...revealedTokens,
      [storeID]: response.data.token,
    };
    successMessage = 'Token toko dirotasi. Token lama langsung tidak berlaku.';
  }

  async function submitDeleteStore(storeID: string) {
    if (!window.confirm('Soft delete toko ini? Token dan relasi staff tetap tercatat di audit.')) {
      return;
    }

    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await deleteStore(storeID);
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }
    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    successMessage = 'Toko berhasil di-soft delete.';
    await loadStoreDirectory();
  }

  async function submitCreateEmployee() {
    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await createEmployee({
      email: createEmployeeForm.email.trim(),
      username: createEmployeeForm.username.trim(),
      password: createEmployeeForm.password,
    });
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }
    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    createEmployeeForm = { email: '', username: '', password: '' };
    successMessage = 'Akun karyawan berhasil dibuat.';
    employeePage = 1;
    lastEmployeePageKey = '';
    await loadEmployeeDirectory();
  }

  async function submitAssignStaff(storeID: string) {
    const form = storeForms[storeID];
    if (!form || form.assign_user_id === '') {
      return;
    }

    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await assignStoreStaff(storeID, form.assign_user_id);
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }
    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    storeForms = {
      ...storeForms,
      [storeID]: {
        ...storeForms[storeID],
        assign_user_id: '',
      },
    };
    successMessage = 'Karyawan berhasil di-assign ke toko.';
    await loadVisibleStoreStaff();
  }

  async function submitUnassignStaff(storeID: string, userID: string) {
    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await unassignStoreStaff(storeID, userID);
    busy = false;

    if (!(await ensureAuthorized(response.message))) {
      return;
    }
    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    successMessage = 'Relasi karyawan dengan toko dilepas.';
    await loadVisibleStoreStaff();
  }

  async function applyStoreFilters() {
    storePage = 1;
    lastStorePageKey = '';
    await loadStoreDirectory();
  }

  async function resetStoreFilters() {
    storeSearchTerm = '';
    storeStatusFilter = 'all';
    lowBalanceFilter = 'all';
    storeCreatedFrom = '';
    storeCreatedTo = '';
    staffSearchTerm = '';
    staffAssignedFrom = '';
    staffAssignedTo = '';
    storePage = 1;
    lastStorePageKey = '';
    await loadStoreDirectory();
  }

  async function applyEmployeeFilters() {
    employeePage = 1;
    lastEmployeePageKey = '';
    await loadEmployeeDirectory();
  }

  async function resetEmployeeFilters() {
    employeeSearchTerm = '';
    employeeCreatedFrom = '';
    employeeCreatedTo = '';
    employeePage = 1;
    lastEmployeePageKey = '';
    await loadEmployeeDirectory();
  }

  async function applyStaffFilters() {
    await loadVisibleStoreStaff();
  }

  async function resetStaffFilters() {
    staffSearchTerm = '';
    staffAssignedFrom = '';
    staffAssignedTo = '';
    await loadVisibleStoreStaff();
  }

  async function ensureAuthorized(message: string) {
    if (message !== 'UNAUTHORIZED') {
      return true;
    }

    await goto('/login');
    return false;
  }

  function normalizeOptional(value: string) {
    const trimmed = value.trim();
    return trimmed === '' ? undefined : trimmed;
  }

  function currentRole() {
    return $authSession?.user.role ?? '';
  }

  function canManageStores() {
    return ['owner', 'dev', 'superadmin'].includes(currentRole());
  }

  function canRotateTokens() {
    return ['owner', 'superadmin'].includes(currentRole());
  }

  function canManageEmployees() {
    return currentRole() === 'owner';
  }

  function canViewStaffLists() {
    return currentRole() !== 'karyawan';
  }

  function statusOptions() {
    if (currentRole() === 'owner') {
      return ['active', 'inactive'];
    }

    return ['active', 'inactive', 'banned'];
  }

  function toMessage(message: string) {
    switch (message) {
      case 'UNAUTHORIZED':
        return 'Sesi dashboard berakhir. Silakan login ulang.';
      case 'FORBIDDEN':
        return 'Aksi ini tidak tersedia untuk role Anda.';
      case 'NOT_FOUND':
        return 'Data yang diminta tidak ditemukan.';
      case 'INVALID_STORE_NAME':
        return 'Nama toko wajib diisi.';
      case 'INVALID_SLUG':
        return 'Slug harus lowercase dan hanya boleh berisi huruf kecil, angka, serta dash.';
      case 'DUPLICATE_SLUG':
        return 'Slug toko sudah dipakai toko lain.';
      case 'INVALID_THRESHOLD':
        return 'Low balance threshold harus angka nol atau lebih besar.';
      case 'INVALID_STATUS':
        return 'Status toko tidak valid untuk request ini.';
      case 'INVALID_CALLBACK_URL':
        return 'Callback URL harus memakai http atau https yang valid.';
      case 'INVALID_EMPLOYEE_INPUT':
        return 'Email, username, dan password karyawan wajib diisi.';
      case 'DUPLICATE_IDENTITY':
        return 'Email atau username karyawan sudah dipakai.';
      case 'STAFF_ALREADY_ASSIGNED':
        return 'Karyawan tersebut sudah terhubung ke toko ini.';
      case 'CROSS_OWNER_RELATION_FORBIDDEN':
        return 'Karyawan hanya bisa dihubungkan ke toko milik owner yang sama.';
      default:
        return 'Terjadi kesalahan. Coba ulangi.';
    }
  }

  function exportStoresToCSV() {
    exportRowsToCSV(
      'stores-directory-page',
      [
        { label: 'Name', value: (store) => store.name },
        { label: 'Slug', value: (store) => store.slug },
        { label: 'Status', value: (store) => store.status },
        { label: 'Current Balance', value: (store) => store.current_balance },
        { label: 'Low Balance Threshold', value: (store) => store.low_balance_threshold ?? '-' },
        { label: 'Staff Count', value: (store) => store.staff_count },
        { label: 'Callback URL', value: (store) => store.callback_url || '-' },
        { label: 'Created At', value: (store) => formatDateTime(store.created_at) },
      ],
      stores,
    );
  }

  function exportStoresToXLSX() {
    return exportRowsToXLSX(
      'stores-directory-page',
      'Stores',
      [
        { label: 'Name', value: (store) => store.name },
        { label: 'Slug', value: (store) => store.slug },
        { label: 'Status', value: (store) => store.status },
        { label: 'Current Balance', value: (store) => store.current_balance },
        { label: 'Low Balance Threshold', value: (store) => store.low_balance_threshold ?? '-' },
        { label: 'Staff Count', value: (store) => store.staff_count },
        { label: 'Callback URL', value: (store) => store.callback_url || '-' },
        { label: 'Created At', value: (store) => formatDateTime(store.created_at) },
      ],
      stores,
    );
  }

  function exportStoresToPDF() {
    return exportRowsToPDF(
      'stores-directory-page',
      'Store Directory (Current Page)',
      [
        { label: 'Store', value: (store) => store.name },
        { label: 'Slug', value: (store) => store.slug },
        { label: 'Status', value: (store) => store.status },
        { label: 'Balance', value: (store) => formatCurrency(store.current_balance) },
        { label: 'Threshold', value: (store) => formatCurrency(store.low_balance_threshold) },
        { label: 'Staff', value: (store) => String(store.staff_count) },
      ],
      stores,
    );
  }

  function exportEmployeesToCSV() {
    exportRowsToCSV(
      'employee-directory-page',
      [
        { label: 'Username', value: (user) => user.username },
        { label: 'Email', value: (user) => user.email },
        { label: 'Role', value: (user) => user.role },
        { label: 'Created At', value: (user) => formatDateTime(user.created_at) },
        { label: 'Last Login', value: (user) => formatDateTime(user.last_login_at) },
      ],
      employees,
    );
  }

  function exportEmployeesToXLSX() {
    return exportRowsToXLSX(
      'employee-directory-page',
      'Employees',
      [
        { label: 'Username', value: (user) => user.username },
        { label: 'Email', value: (user) => user.email },
        { label: 'Role', value: (user) => user.role },
        { label: 'Created At', value: (user) => formatDateTime(user.created_at) },
        { label: 'Last Login', value: (user) => formatDateTime(user.last_login_at) },
      ],
      employees,
    );
  }

  function exportEmployeesToPDF() {
    return exportRowsToPDF(
      'employee-directory-page',
      'Employee Directory (Current Page)',
      [
        { label: 'Username', value: (user) => user.username },
        { label: 'Email', value: (user) => user.email },
        { label: 'Role', value: (user) => user.role },
        { label: 'Created', value: (user) => formatDateTime(user.created_at) },
        { label: 'Last Login', value: (user) => formatDateTime(user.last_login_at) },
      ],
      employees,
    );
  }
</script>

<svelte:head>
  <title>Stores | onixggr</title>
</svelte:head>

{#if loading}
  <div class="glass-panel rounded-[2.5rem] p-6">
    <p class="text-sm text-ink-700">
      Memuat store directory, token integrasi, callback rail, dan sub-surface staff...
    </p>
  </div>
{:else}
  <div class="space-y-6">
    <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
      <div class="grid gap-6 xl:grid-cols-[1.08fr_0.92fr]">
        <div class="space-y-4">
          <div class="flex flex-wrap gap-3">
            <span class="status-chip">store control plane</span>
            <span class="status-chip">{role || 'guest'}</span>
            <span class="status-chip">{formatNumber(storeSummary.total_count)} tenant row</span>
          </div>
          <div class="space-y-3">
            <p class="section-kicker">Store operations</p>
            <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
              Token, callback, saldo risk, dan staff scope dalam satu command deck.
            </h1>
            <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
              Halaman ini sekarang membaca store directory, employee roster, dan staff preview
              langsung dari backend dengan pagination dan filter server-side supaya tetap ringan saat
              data sudah besar.
            </p>
          </div>
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <MetricCard
            class="h-full"
            eyebrow="Directory"
            title="Visible page"
            value={formatNumber(visibleStoreCount)}
            detail={`Total terfilter ${formatNumber(storeSummary.total_count)} toko tersedia dari backend.`}
            tone="brand"
          />
          <MetricCard
            class="h-full"
            eyebrow="Risk"
            title="Low balance"
            value={formatNumber(storeSummary.low_balance_count)}
            detail={`Token baru yang terekspos di sesi ini: ${formatNumber(revealedTokenCount)}.`}
            tone={storeSummary.low_balance_count > 0 ? 'accent' : 'default'}
          />
        </div>
      </div>
    </section>

    {#if errorMessage !== ''}
      <Notice tone="error" title="Store Action Error" message={errorMessage} />
    {/if}

    {#if successMessage !== ''}
      <Notice tone="success" title="Store Action Success" message={successMessage} />
    {/if}

    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <MetricCard
        eyebrow="Total"
        title="Stores matched"
        value={formatNumber(storeSummary.total_count)}
        detail="Total row toko setelah filter store aktif diterapkan di backend."
        tone="brand"
      />
      <MetricCard
        eyebrow="Live"
        title="Active stores"
        value={formatNumber(storeSummary.active_count)}
        detail="Store active yang masih tersedia dalam query saat ini."
      />
      <MetricCard
        eyebrow="Workforce"
        title="Visible staff"
        value={formatNumber(visibleStaffCount)}
        detail={`Preview staff dari ${formatNumber(totalAssignedStaffCount)} relasi yang cocok di current page.`}
      />
      <MetricCard
        eyebrow="Secrets"
        title="One-time reveals"
        value={formatNumber(revealedTokenCount)}
        detail="Token hanya tampil sekali setelah create atau rotate."
        tone="accent"
      />
    </div>

    <div class="grid gap-6 xl:grid-cols-[0.95fr_1.05fr]">
      <section class="space-y-6">
        {#if canManageEmployees()}
          <div class="grid gap-6 xl:grid-cols-[1.02fr_0.98fr]">
            <section class="glass-panel rounded-[2.2rem] p-6">
              <p class="section-kicker !text-brand-700">Provision store</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Buat tenant baru
              </h2>
              <p class="mt-3 text-sm leading-7 text-ink-700">
                Store baru otomatis mendapat ledger account dan one-time token integration.
              </p>

              <div class="mt-5 grid gap-4 md:grid-cols-2">
                <label class="field-stack">
                  <span class="field-label">Nama toko</span>
                  <input bind:value={createStoreForm.name} class="field-input" placeholder="Alpha Store" />
                </label>

                <label class="field-stack">
                  <span class="field-label">Slug</span>
                  <input bind:value={createStoreForm.slug} class="field-input" placeholder="alpha-store" />
                </label>

                <label class="field-stack md:col-span-2">
                  <span class="field-label">Low balance threshold</span>
                  <input bind:value={createStoreForm.low_balance_threshold} class="field-input" inputmode="decimal" placeholder="150000" />
                </label>
              </div>

              <div class="mt-5">
                <Button variant="brand" size="lg" onclick={submitCreateStore} disabled={busy}>
                  Buat Toko
                </Button>
              </div>
            </section>

            <section class="glass-panel rounded-[2.2rem] p-6">
              <p class="section-kicker !text-brand-700">Provision staff</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Buat akun karyawan
              </h2>
              <p class="mt-3 text-sm leading-7 text-ink-700">
                Owner membuat akun karyawan dulu, baru melakukan assignment ke store yang relevan.
              </p>

              <div class="mt-5 space-y-4">
                <label class="field-stack">
                  <span class="field-label">Email</span>
                  <input bind:value={createEmployeeForm.email} class="field-input" placeholder="staff@example.com" />
                </label>

                <label class="field-stack">
                  <span class="field-label">Username</span>
                  <input bind:value={createEmployeeForm.username} class="field-input" placeholder="staff-alpha" />
                </label>

                <label class="field-stack">
                  <span class="field-label">Password awal</span>
                  <input bind:value={createEmployeeForm.password} class="field-input" type="password" placeholder="StaffDemo123!" />
                </label>
              </div>

              <div class="mt-5">
                <Button variant="outline" size="lg" class="w-full" onclick={submitCreateEmployee} disabled={busy}>
                  Buat Akun Karyawan
                </Button>
              </div>
            </section>
          </div>

          <section class="glass-panel rounded-[2.2rem] p-6">
            <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
              <div>
                <p class="section-kicker !text-brand-700">Employee directory</p>
                <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                  Owner staff roster
                </h2>
                <p class="mt-3 text-sm leading-7 text-ink-700">
                  Search, date filter, export, dan pagination untuk akun karyawan milik owner saat
                  ini.
                </p>
              </div>

              <ExportActions
                count={employees.length}
                disabled={employees.length === 0}
                onCsv={exportEmployeesToCSV}
                onXlsx={exportEmployeesToXLSX}
                onPdf={exportEmployeesToPDF}
              />
            </div>

            <div class="mt-5 grid gap-4 2xl:grid-cols-[minmax(0,1fr)_minmax(18rem,22rem)]">
              <label class="field-stack">
                <span class="field-label">Cari karyawan</span>
                <input bind:value={employeeSearchTerm} class="field-input" placeholder="Cari username atau email" />
              </label>

              <DateRangeFilter bind:start={employeeCreatedFrom} bind:end={employeeCreatedTo} label="Employee created" />
            </div>

            <div class="mt-4 flex flex-wrap gap-3">
              <Button variant="brand" size="sm" onclick={applyEmployeeFilters} disabled={employeeDirectoryLoading}>
                Apply Filters
              </Button>
              <Button variant="outline" size="sm" onclick={resetEmployeeFilters} disabled={employeeDirectoryLoading}>
                Reset
              </Button>
            </div>

            <div class="mt-5 space-y-3">
              {#if employeeDirectoryLoading}
                <div class="rounded-[1.6rem] bg-canvas-50 px-4 py-4 text-sm text-ink-700">
                  Memuat employee directory...
                </div>
              {:else if employees.length === 0}
                <EmptyState
                  eyebrow="Employee Directory"
                  title="Belum ada karyawan yang cocok"
                  body="Akun karyawan owner akan tampil di sini setelah dibuat dan sesuai filter aktif."
                />
              {:else}
                <div class="grid gap-3 xl:grid-cols-2">
                  {#each employees as employee}
                    <article class="rounded-[1.6rem] border border-ink-100 bg-white/84 px-4 py-4">
                      <div class="flex items-start justify-between gap-3">
                        <div class="space-y-1">
                          <p class="font-semibold text-ink-900">{employee.username}</p>
                          <p class="text-sm text-ink-700">{employee.email}</p>
                        </div>
                        <span class="surface-chip">{employee.role}</span>
                      </div>
                      <p class="mt-3 text-xs leading-5 text-ink-500">
                        Dibuat {formatDateTime(employee.created_at)} · Last login {employee.last_login_at
                          ? formatDateTime(employee.last_login_at)
                          : 'belum pernah login'}
                      </p>
                    </article>
                  {/each}
                </div>

                <div class="mt-5">
                  <PaginationControls bind:page={employeePage} bind:pageSize={employeePageSize} totalItems={employeeTotalCount} pageSizeOptions={[6, 12, 24, 48]} />
                </div>
              {/if}
            </div>
          </section>
        {:else if currentRole() === 'dev' || currentRole() === 'superadmin'}
          <section class="glass-panel rounded-[2.2rem] p-6">
            <p class="section-kicker !text-brand-700">Owner onboarding</p>
            <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
              Owner dibuat dari Users, lalu owner membuat store di sini
            </h2>
            <p class="mt-3 text-sm leading-7 text-ink-700">
              Halaman Stores tetap menjadi tempat owner membuat tenant pertamanya. Dev atau
              superadmin sekarang mendaftarkan owner dulu dari halaman Users, lalu owner login dan
              menerbitkan token awal saat create store.
            </p>

            <div class="mt-5 grid gap-3 sm:grid-cols-2">
              <div class="rounded-[1.5rem] bg-canvas-50 px-4 py-4">
                <p class="text-sm font-semibold text-ink-900">1. Provision owner</p>
                <p class="mt-2 text-sm leading-6 text-ink-700">
                  Gunakan halaman Users untuk membuat owner, mengaktifkan ulang akun, dan menjaga
                  onboarding tetap tercatat di audit.
                </p>
              </div>
              <div class="rounded-[1.5rem] bg-canvas-50 px-4 py-4">
                <p class="text-sm font-semibold text-ink-900">2. Owner builds tenant</p>
                <p class="mt-2 text-sm leading-6 text-ink-700">
                  Setelah login, owner membuat store di surface ini, lalu lanjut ke API Docs untuk
                  integrasi website.
                </p>
              </div>
            </div>

            <div class="mt-5 flex flex-wrap gap-3">
              <a class="surface-chip" href="/app/users">Open Users</a>
              <a class="surface-chip" href="/app/api-docs">Open API Docs</a>
            </div>
          </section>
        {:else}
          <section class="glass-panel rounded-[2.2rem] p-6">
            <p class="section-kicker !text-brand-700">Store scope</p>
            <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
              Karyawan hanya membaca store yang sudah di-scope
            </h2>
            <p class="mt-3 text-sm leading-7 text-ink-700">
              Surface ini untuk karyawan fokus ke store yang memang sudah diassign. Provision owner,
              create store, dan create staff tetap dilakukan oleh owner atau role platform sesuai
              boundary blueprint.
            </p>
          </section>
        {/if}
      </section>

      <section class="glass-panel rounded-[2.2rem] p-6">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p class="section-kicker !text-brand-700">Store directory</p>
            <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
              Filtered roster
            </h2>
            <p class="mt-3 text-sm leading-7 text-ink-700">
              Semua toko dalam scope aktif, dengan token reveal, callback rail, low-balance signal,
              dan preview staff per page.
            </p>
          </div>

          <ExportActions
            count={stores.length}
            disabled={stores.length === 0}
            onCsv={exportStoresToCSV}
            onXlsx={exportStoresToXLSX}
            onPdf={exportStoresToPDF}
          />
        </div>

        <div class="mt-5 grid gap-4 2xl:grid-cols-[12rem_12rem_minmax(0,1fr)]">
          <label class="field-stack">
            <span class="field-label">Status</span>
            <select bind:value={storeStatusFilter} class="field-select">
              <option value="all">Semua status</option>
              <option value="active">Active</option>
              <option value="inactive">Inactive</option>
              {#if currentRole() !== 'owner'}
                <option value="banned">Banned</option>
              {/if}
            </select>
          </label>

          <label class="field-stack">
            <span class="field-label">Saldo risk</span>
            <select bind:value={lowBalanceFilter} class="field-select">
              <option value="all">Semua kondisi</option>
              <option value="low_balance">Low balance</option>
              <option value="healthy">Healthy</option>
            </select>
          </label>

          <label class="field-stack">
            <span class="field-label">Cari store</span>
            <input bind:value={storeSearchTerm} class="field-input" placeholder="Cari nama, slug, atau callback URL" />
          </label>
        </div>

        <div class="mt-4 grid gap-4 2xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <DateRangeFilter bind:start={storeCreatedFrom} bind:end={storeCreatedTo} label="Store created" />
          <div class="grid gap-4">
            <label class="field-stack">
              <span class="field-label">Cari staff preview</span>
              <input bind:value={staffSearchTerm} class="field-input" placeholder="Cari username atau email staff" />
            </label>
            <DateRangeFilter bind:start={staffAssignedFrom} bind:end={staffAssignedTo} label="Staff assigned" />
          </div>
        </div>

        <div class="mt-4 flex flex-wrap gap-3">
          <Button variant="brand" size="sm" onclick={applyStoreFilters} disabled={directoryLoading}>
            Apply Store Filters
          </Button>
          <Button variant="outline" size="sm" onclick={resetStoreFilters} disabled={directoryLoading}>
            Reset Store Filters
          </Button>
          {#if canViewStaffLists()}
            <Button variant="outline" size="sm" onclick={applyStaffFilters} disabled={directoryLoading}>
              Refresh Staff Preview
            </Button>
            <Button variant="outline" size="sm" onclick={resetStaffFilters} disabled={directoryLoading}>
              Reset Staff Preview
            </Button>
          {/if}
        </div>

        <div class="mt-5 space-y-5">
          {#if directoryLoading}
            <div class="rounded-[1.7rem] bg-canvas-50 px-4 py-4 text-sm text-ink-700">
              Memuat store directory dari backend...
            </div>
          {:else if storeSummary.total_count === 0}
            <EmptyState
              eyebrow="Store Directory"
              title="Belum ada store yang cocok"
              body="Ubah filter atau buat tenant baru agar roster toko muncul di sini."
            />
          {:else}
            {#each stores as store}
              <article class="rounded-[2rem] border border-ink-100 bg-white/82 p-5 shadow-[0_18px_38px_rgba(7,16,12,0.08)]">
                <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                  <div class="space-y-2">
                    <p class="text-xs font-semibold uppercase tracking-[0.24em] text-accent-700">
                      {store.slug}
                    </p>
                    <h3 class="font-display text-2xl font-bold tracking-tight text-ink-900">
                      {store.name}
                    </h3>
                    <div class="flex flex-wrap gap-2">
                      <span class="surface-chip">{store.status}</span>
                      {#if isStoreLowBalance(store)}
                        <span class="surface-chip">low balance</span>
                      {/if}
                      <span class="surface-chip">{formatNumber(store.staff_count)} staff</span>
                    </div>
                  </div>

                  <div class="rounded-[1.6rem] bg-canvas-50 px-4 py-4 text-sm text-ink-700">
                    <p class="font-semibold text-ink-900">Store meta</p>
                    <p class="mt-2">Balance: {formatCurrency(store.current_balance)}</p>
                    <p>Threshold: {formatCurrency(store.low_balance_threshold)}</p>
                    <p>Created: {formatDateTime(store.created_at)}</p>
                  </div>
                </div>

                {#if revealedTokens[store.id]}
                  <div class="mt-5 rounded-[1.7rem] border border-brand-200 bg-brand-100/60 px-4 py-4">
                    <p class="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-brand-700">
                      One-time token reveal
                    </p>
                    <p class="mt-2 break-all font-mono text-sm text-ink-900">{revealedTokens[store.id]}</p>
                  </div>
                {/if}

                {#if canManageStores() && storeForms[store.id]}
                  <div class="mt-5 grid gap-4 md:grid-cols-2">
                    <label class="field-stack">
                      <span class="field-label">Nama toko</span>
                      <input bind:value={storeForms[store.id].name} class="field-input" />
                    </label>

                    <label class="field-stack">
                      <span class="field-label">Status</span>
                      <select bind:value={storeForms[store.id].status} class="field-select">
                        {#each statusOptions() as status}
                          <option value={status}>{status}</option>
                        {/each}
                      </select>
                    </label>

                    <label class="field-stack md:col-span-2">
                      <span class="field-label">Low balance threshold</span>
                      <input bind:value={storeForms[store.id].low_balance_threshold} class="field-input" inputmode="decimal" placeholder="150000" />
                    </label>

                    <label class="field-stack md:col-span-2">
                      <span class="field-label">Callback URL</span>
                      <input bind:value={storeForms[store.id].callback_url} class="field-input" placeholder="https://merchant.example.com/callback" />
                    </label>
                  </div>

                  <div class="mt-5 flex flex-wrap gap-3">
                    <Button variant="brand" size="lg" onclick={() => submitStoreUpdate(store.id)} disabled={busy}>
                      Simpan Toko
                    </Button>
                    <Button variant="outline" size="lg" onclick={() => submitCallbackUpdate(store.id)} disabled={busy}>
                      Simpan Callback
                    </Button>
                    {#if canRotateTokens()}
                      <Button variant="outline" size="lg" onclick={() => submitRotateToken(store.id)} disabled={busy}>
                        Rotate Token
                      </Button>
                    {/if}
                    <Button variant="outline" size="lg" onclick={() => submitDeleteStore(store.id)} disabled={busy}>
                      Soft Delete
                    </Button>
                  </div>
                {:else}
                  <div class="mt-5 rounded-[1.6rem] border border-ink-100 bg-canvas-50 px-4 py-4 text-sm text-ink-700">
                    <p class="font-semibold text-ink-900">Callback URL</p>
                    <p class="mt-2 break-all">{store.callback_url || 'Disembunyikan untuk role ini'}</p>
                  </div>
                {/if}

                {#if canViewStaffLists()}
                  <div class="mt-6 rounded-[1.8rem] border border-ink-100 bg-canvas-50/70 p-4">
                    <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                      <div>
                        <p class="font-semibold text-ink-900">Staff scope</p>
                        <p class="mt-1 text-sm text-ink-700">
                          Menampilkan {formatNumber((staffByStore[store.id] ?? []).length)} dari {formatNumber(staffCountByStore[store.id] ?? 0)} assignment yang cocok pada current page preview.
                        </p>
                      </div>

                      {#if canManageEmployees() && visibleEmployeeOptions.length > 0 && storeForms[store.id]}
                        <div class="flex w-full flex-col gap-3 xl:max-w-[24rem] xl:flex-row">
                          <select bind:value={storeForms[store.id].assign_user_id} class="field-select">
                            <option value="">Pilih karyawan di current employee page</option>
                            {#each visibleEmployeeOptions as employee}
                              <option value={employee.id}>{employee.username} · {employee.email}</option>
                            {/each}
                          </select>
                          <Button variant="outline" size="lg" onclick={() => submitAssignStaff(store.id)} disabled={busy}>
                            Assign
                          </Button>
                        </div>
                      {/if}
                    </div>

                    <div class="mt-4 space-y-3">
                      {#if (staffByStore[store.id] ?? []).length === 0}
                        <div class="rounded-[1.4rem] bg-white px-4 py-4 text-sm text-ink-700">
                          Belum ada staff yang cocok dengan preview filter saat ini.
                        </div>
                      {:else}
                        {#each staffByStore[store.id] ?? [] as user}
                          <article class="flex flex-col gap-3 rounded-[1.4rem] border border-white/60 bg-white px-4 py-4 md:flex-row md:items-center md:justify-between">
                            <div class="space-y-1 text-sm text-ink-700">
                              <p class="font-semibold text-ink-900">{user.username}</p>
                              <p>{user.email}</p>
                              <p class="text-xs text-ink-500">
                                Assigned {user.assigned_at ? formatDateTime(user.assigned_at) : '-'} · Last login {user.last_login_at
                                  ? formatDateTime(user.last_login_at)
                                  : 'belum pernah login'}
                              </p>
                            </div>

                            {#if canManageEmployees()}
                              <Button variant="outline" size="lg" onclick={() => submitUnassignStaff(store.id, user.id)} disabled={busy}>
                                Unassign
                              </Button>
                            {/if}
                          </article>
                        {/each}
                      {/if}
                    </div>
                  </div>
                {/if}
              </article>
            {/each}

            <PaginationControls bind:page={storePage} bind:pageSize={storePageSize} totalItems={storeTotalCount} pageSizeOptions={[4, 8, 12, 24]} />
          {/if}
        </div>
      </section>
    </div>
  </div>
{/if}
