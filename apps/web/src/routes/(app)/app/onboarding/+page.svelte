<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import { authSession, initializeAuthSession } from '$lib/auth/client';
  import EmptyState from '$lib/components/app/empty-state.svelte';
  import MetricCard from '$lib/components/app/metric-card.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import Button from '$lib/components/ui/button/button.svelte';
  import { formatDateTime, formatNumber } from '$lib/formatters';
  import {
    createStore,
    fetchStoreDirectory,
    type Store,
    type StoreDirectorySummary,
  } from '$lib/stores/client';
  import {
    createManagedUser,
    fetchUserDirectory,
    type ManagedUser,
    type UserDirectorySummary,
  } from '$lib/users/client';

  const emptyUserSummary: UserDirectorySummary = {
    total_count: 0,
    owner_count: 0,
    superadmin_count: 0,
    dev_count: 0,
    active_count: 0,
    inactive_count: 0,
  };

  const emptyStoreSummary: StoreDirectorySummary = {
    total_count: 0,
    active_count: 0,
    inactive_count: 0,
    banned_count: 0,
    deleted_count: 0,
    low_balance_count: 0,
  };

  let loading = true;
  let busy = false;
  let errorMessage = '';
  let successMessage = '';

  let ownerUsers: ManagedUser[] = [];
  let userSummary: UserDirectorySummary = { ...emptyUserSummary };
  let stores: Store[] = [];
  let storeSummary: StoreDirectorySummary = { ...emptyStoreSummary };
  let revealedStoreToken = '';

  let ownerForm = {
    email: '',
    username: '',
    password: '',
  };

  let ownerStoreForm = {
    name: '',
    slug: '',
    low_balance_threshold: '',
  };

  $: role = $authSession?.user.role ?? '';
  $: isPlatformRole = role === 'dev' || role === 'superadmin';
  $: isOwnerRole = role === 'owner';
  $: isEmployeeRole = role === 'karyawan';
  $: canCreateOwner = role === 'dev' || role === 'superadmin';
  $: canCreateStore = role === 'owner';

  onMount(async () => {
    await initializeAuthSession();

    if (!$authSession) {
      await goto('/login');
      return;
    }

    await loadWorkspace();
  });

  async function loadWorkspace() {
    loading = true;
    errorMessage = '';

    const tasks: Promise<unknown>[] = [loadStores()];
    if (isPlatformRole) {
      tasks.push(loadOwners());
    }

    await Promise.all(tasks);
    loading = false;
  }

  async function loadOwners() {
    const response = await fetchUserDirectory({
      role: 'owner',
      limit: 6,
      offset: 0,
    });

    if (!response.status || response.message !== 'SUCCESS') {
      if (response.message === 'FORBIDDEN') {
        return;
      }

      errorMessage = toMessage(response.message);
      return;
    }

    ownerUsers = response.data.items ?? [];
    userSummary = response.data.summary ?? { ...emptyUserSummary };
  }

  async function loadStores() {
    const response = await fetchStoreDirectory({
      limit: 6,
      offset: 0,
    });

    if (!response.status || response.message !== 'SUCCESS') {
      if (response.message === 'FORBIDDEN') {
        stores = [];
        storeSummary = { ...emptyStoreSummary };
        return;
      }

      errorMessage = toMessage(response.message);
      return;
    }

    stores = response.data.items ?? [];
    storeSummary = response.data.summary ?? { ...emptyStoreSummary };
  }

  async function submitOwnerProvision() {
    if (!canCreateOwner) {
      return;
    }

    busy = true;
    errorMessage = '';
    successMessage = '';

    const response = await createManagedUser({
      email: ownerForm.email.trim(),
      username: ownerForm.username.trim(),
      password: ownerForm.password,
      role: 'owner',
    });
    busy = false;

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    ownerForm = {
      email: '',
      username: '',
      password: '',
    };
    successMessage =
      'Owner berhasil dibuat. Minta owner login lalu buka onboarding ini atau halaman Stores untuk menerbitkan tenant pertama.';
    await loadWorkspace();
  }

  async function submitCreateOwnerStore() {
    if (!canCreateStore) {
      return;
    }

    busy = true;
    errorMessage = '';
    successMessage = '';
    revealedStoreToken = '';

    const response = await createStore({
      name: ownerStoreForm.name.trim(),
      slug: ownerStoreForm.slug.trim(),
      low_balance_threshold:
        ownerStoreForm.low_balance_threshold.trim() === ''
          ? undefined
          : ownerStoreForm.low_balance_threshold.trim(),
    });
    busy = false;

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    ownerStoreForm = {
      name: '',
      slug: '',
      low_balance_threshold: '',
    };
    revealedStoreToken = response.data.api_token ?? '';
    successMessage =
      'Store berhasil dibuat. Simpan token one-time reveal ini sekarang, lalu lanjutkan callback URL dan integrasi website.';
    await loadWorkspace();
  }

  function toMessage(message: string) {
    switch (message) {
      case 'UNAUTHORIZED':
        return 'Sesi dashboard berakhir. Silakan login ulang.';
      case 'FORBIDDEN':
        return 'Surface ini tidak tersedia untuk role saat ini.';
      case 'INVALID_INPUT':
        return 'Form belum valid. Periksa email, username, password, dan field wajib lain.';
      case 'DUPLICATE_IDENTITY':
        return 'Email atau username sudah dipakai.';
      case 'DUPLICATE_SLUG':
        return 'Slug toko sudah dipakai.';
      case 'ROLE_PROVISION_FORBIDDEN':
        return 'Role saat ini tidak boleh memprovisi owner.';
      default:
        return 'Terjadi kesalahan. Coba ulangi beberapa saat lagi.';
    }
  }
</script>

<svelte:head>
  <title>Onboarding | onixggr</title>
</svelte:head>

{#if loading}
  <div class="glass-panel rounded-[2.3rem] p-6">
    <p class="text-sm text-ink-700">Memuat flow onboarding dev, owner, tenant, dan integrasi...</p>
  </div>
{:else if isEmployeeRole}
  <EmptyState
    eyebrow="Onboarding"
    title="Karyawan tidak menjalankan onboarding tenant"
    body="Role karyawan hanya bekerja pada store yang sudah diassign. Provision owner, create store, dan token awal dilakukan oleh dev/superadmin atau owner."
    actionHref="/app/stores"
    actionLabel="Buka Stores"
  />
{:else}
  <div class="space-y-6">
    <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
      <div class="grid gap-6 xl:grid-cols-[1.08fr_0.92fr]">
        <div class="space-y-4">
          <div class="flex flex-wrap gap-3">
            <span class="status-chip">tenant onboarding</span>
            <span class="status-chip">{role || 'guest'}</span>
            <span class="status-chip">{formatNumber(storeSummary.total_count)} tenant visible</span>
          </div>

          <div class="space-y-3">
            <p class="section-kicker">Operator runway</p>
            <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
              Jalur resmi dari dev ke owner, lalu owner ke store dan website integration.
            </h1>
            <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
              Flow ini mengikuti blueprint: role platform memprovisi owner, owner login, owner
              membuat store, token awal keluar satu kali, lalu integrasi berlanjut di API Docs.
            </p>
          </div>
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <MetricCard
            class="h-full"
            eyebrow="Owners"
            title="Owner roster"
            value={formatNumber(userSummary.owner_count)}
            detail="Akun owner aktif yang siap masuk ke tahap create store."
            tone="brand"
          />
          <MetricCard
            class="h-full"
            eyebrow="Stores"
            title="Visible tenants"
            value={formatNumber(storeSummary.total_count)}
            detail="Tenant yang sudah dibuat dan bisa dipantau dari sesi ini."
          />
        </div>
      </div>
    </section>

    {#if errorMessage !== ''}
      <Notice tone="error" title="Onboarding Error" message={errorMessage} />
    {/if}

    {#if successMessage !== ''}
      <Notice tone="success" title="Onboarding Success" message={successMessage} />
    {/if}

    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <MetricCard
        eyebrow="Step 01"
        title="Provision owner"
        value={isPlatformRole ? 'Live' : 'Read-only'}
        detail="Dilakukan oleh dev atau superadmin dari dashboard, bukan lewat SQL manual."
        tone="brand"
      />
      <MetricCard
        eyebrow="Step 02"
        title="Owner login"
        value="Required"
        detail="Ownership token dan callback setup dimulai dari sesi owner yang valid."
      />
      <MetricCard
        eyebrow="Step 03"
        title="Create store"
        value={canCreateStore ? 'You can do this' : 'Owner only'}
        detail="Store pertama menerbitkan token integrasi satu kali saat dibuat."
      />
      <MetricCard
        eyebrow="Step 04"
        title="Integrate website"
        value="API Docs"
        detail="Store API, callback HMAC, dan contoh cURL tersedia setelah tenant siap."
        tone="accent"
      />
    </div>

    <div class="grid gap-6 xl:grid-cols-[1fr_1fr]">
      <section class="form-surface">
        {#if isPlatformRole}
          <div class="space-y-3">
            <p class="section-kicker !text-brand-700">Platform action</p>
            <h2 class="font-display text-3xl font-bold tracking-tight text-ink-900">
              Provision owner baru
            </h2>
            <p class="text-sm leading-7 text-ink-700">
              Begitu owner dibuat, alur berikutnya adalah login sebagai owner lalu membuat store
              pertama. Halaman Stores dan API Docs akan mengambil alih setelah itu.
            </p>
          </div>

          <div class="form-grid mt-6">
            <label class="field-stack">
              <span class="field-label">Email owner</span>
              <input bind:value={ownerForm.email} class="field-input" placeholder="owner@example.com" />
            </label>

            <label class="field-stack">
              <span class="field-label">Username owner</span>
              <input bind:value={ownerForm.username} class="field-input" placeholder="owner-alpha" />
            </label>

            <label class="field-stack md:col-span-2">
              <span class="field-label">Password awal</span>
              <input bind:value={ownerForm.password} class="field-input" type="password" placeholder="OwnerDemo123!" />
            </label>
          </div>

          <div class="stack-actions mt-6">
            <Button variant="brand" size="lg" onclick={submitOwnerProvision} disabled={busy}>
              Provision Owner
            </Button>
            <a class="surface-chip" href="/app/users">Open full Users control plane</a>
            <a class="surface-chip" href="/app/stores">Monitor tenant directory</a>
          </div>
        {:else}
          <div class="space-y-3">
            <p class="section-kicker !text-brand-700">Owner action</p>
            <h2 class="font-display text-3xl font-bold tracking-tight text-ink-900">
              Buat store pertama dari sesi owner
            </h2>
            <p class="text-sm leading-7 text-ink-700">
              Ini adalah langkah yang tadi hilang dari pengalaman dashboard. Setelah store dibuat,
              token integrasi awal muncul satu kali di sini.
            </p>
          </div>

          <div class="form-grid mt-6">
            <label class="field-stack">
              <span class="field-label">Nama store</span>
              <input bind:value={ownerStoreForm.name} class="field-input" placeholder="Alpha Store" />
            </label>

            <label class="field-stack">
              <span class="field-label">Slug</span>
              <input bind:value={ownerStoreForm.slug} class="field-input" placeholder="alpha-store" />
            </label>

            <label class="field-stack md:col-span-2">
              <span class="field-label">Low balance threshold</span>
              <input
                bind:value={ownerStoreForm.low_balance_threshold}
                class="field-input"
                inputmode="decimal"
                placeholder="150000"
              />
            </label>
          </div>

          <div class="stack-actions mt-6">
            <Button variant="brand" size="lg" onclick={submitCreateOwnerStore} disabled={busy}>
              Create Store
            </Button>
            <a class="surface-chip" href="/app/stores">Open full Stores workspace</a>
            <a class="surface-chip" href="/app/api-docs">Continue to API Docs</a>
          </div>

          {#if revealedStoreToken !== ''}
            <div class="mt-6 rounded-[1.7rem] border border-brand-200 bg-brand-100/70 px-4 py-4">
              <p class="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-brand-700">
                One-time token reveal
              </p>
              <p class="mt-3 break-all font-mono text-sm text-ink-900">{revealedStoreToken}</p>
            </div>
          {/if}
        {/if}
      </section>

      <section class="glass-panel rounded-[2rem] p-6">
        <div class="space-y-3">
          <p class="section-kicker !text-brand-700">Runbook</p>
          <h2 class="font-display text-3xl font-bold tracking-tight text-ink-900">
            Urutan yang benar
          </h2>
        </div>

        <ol class="command-list mt-5">
          <li>Dev atau superadmin membuat akun owner dari surface onboarding atau Users.</li>
          <li>Owner login ke dashboard memakai akun itu.</li>
          <li>Owner membuat store pertama dan menerima token awal satu kali.</li>
          <li>Owner membuka Stores untuk callback URL, staff, dan pengelolaan tenant.</li>
          <li>Owner membuka API Docs untuk integrasi website dan verifikasi callback.</li>
        </ol>

        <div class="mt-5 grid gap-3 sm:grid-cols-2">
          <a class="rounded-[1.5rem] bg-canvas-50 px-4 py-4 text-sm text-ink-700" href="/app/stores">
            <p class="font-semibold text-ink-900">Stores workspace</p>
            <p class="mt-2 leading-6">
              Token rotate, callback URL, employee assignment, dan store directory.
            </p>
          </a>
          <a class="rounded-[1.5rem] bg-canvas-50 px-4 py-4 text-sm text-ink-700" href="/app/api-docs">
            <p class="font-semibold text-ink-900">API Docs</p>
            <p class="mt-2 leading-6">
              cURL, callback HMAC, flow game, QRIS, dan format response untuk website owner.
            </p>
          </a>
        </div>
      </section>
    </div>

    <div class="grid gap-6 xl:grid-cols-[0.94fr_1.06fr]">
      {#if isPlatformRole}
        <section class="glass-panel rounded-[2rem] p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Owner roster</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Owner yang baru diprovisi
              </h2>
            </div>
            <span class="surface-chip">{formatNumber(ownerUsers.length)} on page</span>
          </div>

          {#if ownerUsers.length === 0}
            <div class="mt-5">
              <EmptyState
                eyebrow="Owner Directory"
                title="Belum ada owner pada page ini"
                body="Provision owner pertama dari form onboarding di atas atau buka Users untuk filter yang lebih lengkap."
              />
            </div>
          {:else}
            <div class="mt-5 grid gap-3">
              {#each ownerUsers as owner}
                <article class="rounded-[1.5rem] border border-ink-100 bg-white/82 px-4 py-4">
                  <div class="flex items-start justify-between gap-3">
                    <div>
                      <p class="font-semibold text-ink-900">{owner.username}</p>
                      <p class="mt-1 text-sm text-ink-700">{owner.email}</p>
                    </div>
                    <span class="surface-chip">{owner.is_active ? 'active' : 'inactive'}</span>
                  </div>
                  <p class="mt-3 text-xs leading-5 text-ink-500">
                    Dibuat {formatDateTime(owner.created_at)} · Last login {owner.last_login_at
                      ? formatDateTime(owner.last_login_at)
                      : 'belum pernah login'}
                  </p>
                </article>
              {/each}
            </div>
          {/if}
        </section>
      {/if}

      <section class="glass-panel rounded-[2rem] p-6">
        <div class="flex items-end justify-between gap-4">
          <div>
            <p class="section-kicker !text-brand-700">Tenant visibility</p>
            <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
              Store yang sudah siap diintegrasikan
            </h2>
          </div>
          <span class="surface-chip">{formatNumber(storeSummary.total_count)} total</span>
        </div>

        {#if stores.length === 0}
          <div class="mt-5">
            <EmptyState
              eyebrow="Store Directory"
              title="Belum ada tenant yang tampil"
              body="Begitu owner membuat store, tenant akan muncul di sini bersama saldo saat ini dan langkah lanjut ke API Docs."
            />
          </div>
        {:else}
          <div class="mt-5 grid gap-3">
            {#each stores as store}
              <article class="rounded-[1.55rem] border border-ink-100 bg-white/82 px-4 py-4">
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <p class="font-semibold text-ink-900">{store.name}</p>
                    <p class="mt-1 text-sm text-ink-700">{store.slug}</p>
                  </div>
                  <span class="surface-chip">{store.status}</span>
                </div>
                <div class="mt-4 flex flex-wrap gap-2 text-xs leading-5 text-ink-500">
                  <span class="surface-chip">{formatNumber(store.staff_count)} staff</span>
                  <span class="surface-chip">created {formatDateTime(store.created_at)}</span>
                </div>
              </article>
            {/each}
          </div>
        {/if}
      </section>
    </div>
  </div>
{/if}
