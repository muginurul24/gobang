<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import Button from '$lib/components/ui/button/button.svelte';
  import { authSession, initializeAuthSession } from '$lib/auth/client';
  import {
    assignStoreStaff,
    createEmployee,
    createStore,
    deleteStore,
    fetchEmployees,
    fetchStores,
    fetchStoreStaff,
    rotateStoreToken,
    type StaffUser,
    type Store,
    unassignStoreStaff,
    updateStore,
    updateStoreCallbackURL
  } from '$lib/stores/client';

  type StoreFormState = {
    name: string;
    status: string;
    low_balance_threshold: string;
    callback_url: string;
    assign_user_id: string;
  };

  let loading = true;
  let busy = false;
  let errorMessage = '';
  let successMessage = '';
  let stores: Store[] = [];
  let employees: StaffUser[] = [];
  let staffByStore: Record<string, StaffUser[]> = {};
  let storeForms: Record<string, StoreFormState> = {};
  let revealedTokens: Record<string, string> = {};
  let createStoreForm = {
    name: '',
    slug: '',
    low_balance_threshold: ''
  };
  let createEmployeeForm = {
    email: '',
    username: '',
    password: ''
  };

  onMount(async () => {
    await initializeAuthSession();

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
    syncStoreForms();

    if (canManageEmployees()) {
      const employeesResponse = await fetchEmployees();
      if (!(await ensureAuthorized(employeesResponse.message))) {
        return;
      }

      if (employeesResponse.status && employeesResponse.message === 'SUCCESS') {
        employees = employeesResponse.data;
      } else {
        errorMessage = toMessage(employeesResponse.message);
      }
    } else {
      employees = [];
    }

    if (canViewStaffLists() && stores.length > 0) {
      const entries = await Promise.all(
        stores.map(async (store) => {
          const response = await fetchStoreStaff(store.id);
          return [store.id, response] as const;
        })
      );

      const nextStaffByStore: Record<string, StaffUser[]> = {};
      for (const [storeID, response] of entries) {
        if (response.status && response.message === 'SUCCESS') {
          nextStaffByStore[storeID] = response.data;
        } else {
          nextStaffByStore[storeID] = [];
        }
      }

      staffByStore = nextStaffByStore;
    } else {
      staffByStore = {};
    }

    loading = false;
  }

  function syncStoreForms() {
    const nextForms: Record<string, StoreFormState> = {};
    for (const store of stores) {
      nextForms[store.id] = {
        name: store.name,
        status: store.status,
        low_balance_threshold: store.low_balance_threshold ?? '',
        callback_url: store.callback_url ?? '',
        assign_user_id: ''
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
      low_balance_threshold: normalizeOptional(createStoreForm.low_balance_threshold)
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
        [response.data.id]: response.data.api_token
      };
    }

    successMessage = 'Toko baru dibuat. Simpan token integrasi jika baru muncul.';
    await loadScreen();
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
      low_balance_threshold: normalizeOptional(form.low_balance_threshold)
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
    await loadScreen();
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
    await loadScreen();
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
      [storeID]: response.data.token
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
    await loadScreen();
  }

  async function submitCreateEmployee() {
    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await createEmployee({
      email: createEmployeeForm.email.trim(),
      username: createEmployeeForm.username.trim(),
      password: createEmployeeForm.password
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
    await loadScreen();
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

    staffByStore = {
      ...staffByStore,
      [storeID]: response.data
    };
    storeForms = {
      ...storeForms,
      [storeID]: {
        ...storeForms[storeID],
        assign_user_id: ''
      }
    };
    successMessage = 'Karyawan berhasil di-assign ke toko.';
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

    staffByStore = {
      ...staffByStore,
      [storeID]: response.data
    };
    successMessage = 'Relasi karyawan dengan toko dilepas.';
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
</script>

<svelte:head>
  <title>Stores | onixggr</title>
</svelte:head>

{#if loading}
  <div class="glass-panel rounded-4xl p-6">
    <p class="text-sm text-ink-700">Memuat domain toko, token integrasi, dan relasi staff...</p>
  </div>
{:else}
  <div class="space-y-6">
    <section class="glass-panel rounded-4xl p-6">
      <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div class="space-y-2">
          <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
            Store Operations
          </p>
          <h1 class="font-display text-3xl font-bold tracking-tight text-ink-900">
            Toko, token integrasi, callback URL, dan staff scope
          </h1>
          <p class="max-w-3xl text-sm leading-6 text-ink-700">
            Halaman ini menutup Hari 10-12 di blueprint: CRUD toko, token toko hashed, callback URL,
            pembuatan karyawan owner, dan relasi many-to-many staff ke toko.
          </p>
        </div>

        <div class="rounded-3xl bg-canvas-100 px-4 py-3 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Scope</p>
          <p>Role: {$authSession?.user.role ?? '-'}</p>
          <p>Toko aktif: {stores.length}</p>
        </div>
      </div>
    </section>

    {#if errorMessage}
      <div class="rounded-3xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
        {errorMessage}
      </div>
    {/if}

    {#if successMessage}
      <div class="rounded-3xl border border-brand-200 bg-brand-100/60 px-4 py-3 text-sm text-brand-700">
        {successMessage}
      </div>
    {/if}

    {#if canManageEmployees()}
      <div class="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
        <section class="glass-panel rounded-4xl p-6">
          <h2 class="font-display text-2xl font-bold text-ink-900">Buat toko baru</h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            Toko baru akan otomatis mendapat token integrasi baru. Simpan token sekali tampil.
          </p>

          <div class="mt-5 grid gap-4 md:grid-cols-2">
            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Nama toko</span>
              <input
                bind:value={createStoreForm.name}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                placeholder="Alpha Store"
              />
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Slug</span>
              <input
                bind:value={createStoreForm.slug}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                placeholder="alpha-store"
              />
            </label>

            <label class="space-y-2 md:col-span-2">
              <span class="text-sm font-medium text-ink-700">Low balance threshold</span>
              <input
                bind:value={createStoreForm.low_balance_threshold}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                inputmode="decimal"
                placeholder="150000"
              />
            </label>
          </div>

          <div class="mt-5">
            <Button variant="brand" size="lg" onclick={submitCreateStore} disabled={busy}>
              Buat Toko
            </Button>
          </div>
        </section>

        <section class="glass-panel rounded-4xl p-6">
          <h2 class="font-display text-2xl font-bold text-ink-900">Buat akun karyawan</h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            Owner membuat akun karyawan lebih dulu, lalu menghubungkannya ke satu atau banyak toko.
          </p>

          <div class="mt-5 space-y-4">
            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Email</span>
              <input
                bind:value={createEmployeeForm.email}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                placeholder="staff@example.com"
              />
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Username</span>
              <input
                bind:value={createEmployeeForm.username}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                placeholder="staff-alpha"
              />
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Password awal</span>
              <input
                bind:value={createEmployeeForm.password}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                type="password"
                placeholder="StaffDemo123!"
              />
            </label>
          </div>

          <div class="mt-5">
            <Button variant="outline" size="lg" class="w-full" onclick={submitCreateEmployee} disabled={busy}>
              Buat Akun Karyawan
            </Button>
          </div>
        </section>
      </div>
    {/if}

    <section class="grid gap-5 xl:grid-cols-2">
      {#each stores as store}
        <article class="glass-panel rounded-4xl p-6">
          <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
            <div class="space-y-2">
              <p class="text-xs font-semibold uppercase tracking-[0.24em] text-accent-700">
                {store.slug}
              </p>
              <h2 class="font-display text-2xl font-bold text-ink-900">{store.name}</h2>
              <p class="text-sm text-ink-700">
                Status: <span class="font-semibold text-ink-900">{store.status}</span>
              </p>
            </div>

            <div class="rounded-3xl bg-canvas-100 px-4 py-3 text-sm text-ink-700">
              <p class="font-semibold text-ink-900">Store Meta</p>
              <p>Balance: {store.current_balance}</p>
              <p>Staff: {store.staff_count}</p>
              <p>Threshold: {store.low_balance_threshold ?? '-'}</p>
            </div>
          </div>

          {#if revealedTokens[store.id]}
            <div class="mt-5 rounded-3xl border border-brand-200 bg-brand-100/60 px-4 py-4">
              <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
                One-Time Token
              </p>
              <p class="mt-2 font-mono text-sm break-all text-ink-900">{revealedTokens[store.id]}</p>
            </div>
          {/if}

          {#if canManageStores() && storeForms[store.id]}
            <div class="mt-5 grid gap-4 md:grid-cols-2">
              <label class="space-y-2">
                <span class="text-sm font-medium text-ink-700">Nama toko</span>
                <input
                  bind:value={storeForms[store.id].name}
                  class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                />
              </label>

              <label class="space-y-2">
                <span class="text-sm font-medium text-ink-700">Status</span>
                <select
                  bind:value={storeForms[store.id].status}
                  class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                >
                  {#each statusOptions() as status}
                    <option value={status}>{status}</option>
                  {/each}
                </select>
              </label>

              <label class="space-y-2 md:col-span-2">
                <span class="text-sm font-medium text-ink-700">Low balance threshold</span>
                <input
                  bind:value={storeForms[store.id].low_balance_threshold}
                  class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                  inputmode="decimal"
                  placeholder="150000"
                />
              </label>

              <label class="space-y-2 md:col-span-2">
                <span class="text-sm font-medium text-ink-700">Callback URL</span>
                <input
                  bind:value={storeForms[store.id].callback_url}
                  class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                  placeholder="https://merchant.example.com/callback"
                />
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
            <div class="mt-5 rounded-3xl border border-ink-100 bg-white px-4 py-4 text-sm text-ink-700">
              <p class="font-semibold text-ink-900">Callback URL</p>
              <p class="mt-1 break-all">{store.callback_url || 'Disembunyikan untuk role ini'}</p>
            </div>
          {/if}

          {#if canViewStaffLists()}
            <div class="mt-6 rounded-3xl border border-ink-100 bg-white p-4">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <p class="font-semibold text-ink-900">Staff toko</p>
                  <p class="text-sm text-ink-700">
                    {staffByStore[store.id]?.length ?? 0} akun karyawan terhubung.
                  </p>
                </div>
              </div>

              {#if canManageEmployees() && employees.length > 0}
                <div class="mt-4 flex flex-col gap-3 md:flex-row">
                  <select
                    bind:value={storeForms[store.id].assign_user_id}
                    class="w-full rounded-2xl border border-ink-100 bg-canvas-50 px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                  >
                    <option value="">Pilih karyawan owner ini</option>
                    {#each employees as employee}
                      <option value={employee.id}>{employee.username} · {employee.email}</option>
                    {/each}
                  </select>

                  <Button variant="outline" size="lg" onclick={() => submitAssignStaff(store.id)} disabled={busy}>
                    Assign
                  </Button>
                </div>
              {/if}

              <div class="mt-4 space-y-3">
                {#if (staffByStore[store.id]?.length ?? 0) === 0}
                  <p class="text-sm text-ink-700">Belum ada staff yang terhubung ke toko ini.</p>
                {:else}
                  {#each staffByStore[store.id] ?? [] as user}
                    <div class="flex flex-col gap-3 rounded-[1.25rem] bg-canvas-50 px-4 py-3 md:flex-row md:items-center md:justify-between">
                      <div class="text-sm text-ink-700">
                        <p class="font-semibold text-ink-900">{user.username}</p>
                        <p>{user.email}</p>
                      </div>

                      {#if canManageEmployees()}
                        <Button variant="outline" size="lg" onclick={() => submitUnassignStaff(store.id, user.id)} disabled={busy}>
                          Unassign
                        </Button>
                      {/if}
                    </div>
                  {/each}
                {/if}
              </div>
            </div>
          {/if}
        </article>
      {/each}
    </section>
  </div>
{/if}
