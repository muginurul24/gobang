<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import Button from '$lib/components/ui/button/button.svelte';
  import { authSession, hydrateAuthSession } from '$lib/auth/client';
  import { fetchAuditLogs, type AuditLogEntry } from '$lib/audit/client';
  import { fetchStores, type Store } from '$lib/stores/client';

  let loading = true;
  let refreshing = false;
  let errorMessage = '';
  let logs: AuditLogEntry[] = [];
  let stores: Store[] = [];
  let selectedStoreID = '';
  let selectedLimit = 50;
  let actionQuery = '';
  let selectedActorRole = '';
  let selectedTargetType = '';

  onMount(async () => {
    hydrateAuthSession();

    if (!$authSession) {
      await goto('/login');
      return;
    }

    await loadAudit();
  });

  async function loadAudit() {
    loading = true;
    errorMessage = '';

    const storesResponse = await fetchStores();
    if (!(await ensureAuthorized(storesResponse.message))) {
      return;
    }

    if (storesResponse.status && storesResponse.message === 'SUCCESS') {
      stores = storesResponse.data;
    }

    const logsResponse = await fetchAuditLogs({
      storeID: selectedStoreID || undefined,
      limit: selectedLimit,
      action: actionQuery || undefined,
      actorRole: selectedActorRole || undefined,
      targetType: selectedTargetType || undefined
    });
    if (!(await ensureAuthorized(logsResponse.message))) {
      return;
    }

    if (!logsResponse.status || logsResponse.message !== 'SUCCESS') {
      errorMessage = toMessage(logsResponse.message);
      logs = [];
      loading = false;
      return;
    }

    logs = logsResponse.data;
    loading = false;
  }

  async function refreshAudit() {
    refreshing = true;
    await loadAudit();
    refreshing = false;
  }

  async function ensureAuthorized(message: string) {
    if (message !== 'UNAUTHORIZED') {
      return true;
    }

    await goto('/login');
    return false;
  }

  function formatPayload(entry: AuditLogEntry) {
    if (!entry.payload_masked) {
      return '{}';
    }

    return JSON.stringify(entry.payload_masked, null, 2);
  }

  function toMessage(message: string) {
    switch (message) {
      case 'FORBIDDEN':
        return 'Audit viewer hanya tersedia untuk owner, superadmin, dan dev.';
      default:
        return 'Gagal memuat audit log.';
    }
  }
</script>

<svelte:head>
  <title>Audit | onixggr</title>
</svelte:head>

{#if loading}
  <div class="glass-panel rounded-4xl p-6">
    <p class="text-sm text-ink-700">Memuat audit log sesuai scope role...</p>
  </div>
{:else}
  <div class="space-y-6">
    <section class="glass-panel rounded-4xl p-6">
      <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div class="space-y-2">
          <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
            Audit Viewer
          </p>
          <h1 class="font-display text-3xl font-bold tracking-tight text-ink-900">
            Audit trail sesuai boundary owner dan platform
          </h1>
          <p class="max-w-3xl text-sm leading-6 text-ink-700">
            Owner hanya melihat domainnya, superadmin/dev melihat global, dan karyawan diblokir dari
            endpoint audit. Audit log dipertahankan maksimal 90 hari lewat retention job scheduler.
          </p>
        </div>

        <div class="rounded-3xl bg-canvas-100 px-4 py-3 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Filter</p>
          <p>Role: {$authSession?.user.role ?? '-'}</p>
          <p>Total row: {logs.length}</p>
        </div>
      </div>
    </section>

    {#if errorMessage}
      <div class="rounded-3xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
        {errorMessage}
      </div>
    {/if}

    <section class="glass-panel rounded-4xl p-6">
      <div class="grid gap-4 xl:grid-cols-[1.2fr_180px_180px_220px_auto]">
        <label class="space-y-2">
          <span class="text-sm font-medium text-ink-700">Filter toko</span>
          <select
            bind:value={selectedStoreID}
            class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
          >
            <option value="">Semua toko dalam scope</option>
            {#each stores as store}
              <option value={store.id}>{store.name} · {store.slug}</option>
            {/each}
          </select>
        </label>

        <label class="space-y-2">
          <span class="text-sm font-medium text-ink-700">Limit</span>
          <select
            bind:value={selectedLimit}
            class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
          >
            <option value={25}>25</option>
            <option value={50}>50</option>
            <option value={100}>100</option>
          </select>
        </label>

        <label class="space-y-2">
          <span class="text-sm font-medium text-ink-700">Actor role</span>
          <select
            bind:value={selectedActorRole}
            class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
          >
            <option value="">Semua role actor</option>
            <option value="owner">owner</option>
            <option value="karyawan">karyawan</option>
            <option value="dev">dev</option>
            <option value="superadmin">superadmin</option>
            <option value="store_api">store_api</option>
            <option value="provider_webhook">provider_webhook</option>
            <option value="system">system</option>
            <option value="guest">guest</option>
          </select>
        </label>

        <label class="space-y-2">
          <span class="text-sm font-medium text-ink-700">Target type</span>
          <select
            bind:value={selectedTargetType}
            class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
          >
            <option value="">Semua target</option>
            <option value="auth">auth</option>
            <option value="user">user</option>
            <option value="store">store</option>
            <option value="store_member">store_member</option>
            <option value="game_transaction">game_transaction</option>
            <option value="qris_transaction">qris_transaction</option>
            <option value="store_withdrawal">store_withdrawal</option>
          </select>
        </label>

        <div class="flex items-end">
          <Button variant="brand" size="lg" onclick={refreshAudit} disabled={refreshing}>
            Refresh Audit
          </Button>
        </div>
      </div>

      <div class="mt-4 grid gap-4 md:grid-cols-[1fr_auto]">
        <label class="space-y-2">
          <span class="text-sm font-medium text-ink-700">Action contains</span>
          <input
            bind:value={actionQuery}
            class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
            placeholder="contoh: withdraw, auth.login, store.token"
            type="text"
          />
        </label>

        <div class="flex items-end">
          <Button
            variant="outline"
            size="lg"
            onclick={() => {
              actionQuery = '';
              selectedActorRole = '';
              selectedTargetType = '';
              selectedStoreID = '';
              selectedLimit = 50;
              void refreshAudit();
            }}
          >
            Reset Filter
          </Button>
        </div>
      </div>
    </section>

    <section class="space-y-4">
      {#if logs.length === 0}
        <div class="glass-panel rounded-4xl p-6 text-sm text-ink-700">
          Tidak ada audit log dalam scope dan filter saat ini.
        </div>
      {:else}
        {#each logs as entry}
          <article class="glass-panel rounded-4xl p-6">
            <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
              <div class="space-y-1">
                <p class="text-xs font-semibold uppercase tracking-[0.24em] text-accent-700">
                  {entry.action}
                </p>
                <h2 class="font-display text-2xl font-bold text-ink-900">{entry.target_type}</h2>
                <p class="text-sm text-ink-700">
                  Actor role: <span class="font-semibold text-ink-900">{entry.actor_role}</span>
                </p>
                <p class="text-xs text-ink-500">Actor ID: {entry.actor_user_id ?? '-'}</p>
              </div>

              <div class="rounded-3xl bg-canvas-100 px-4 py-3 text-sm text-ink-700">
                <p class="font-semibold text-ink-900">Meta</p>
                <p>Store: {entry.store_id ?? '-'}</p>
                <p>Target: {entry.target_id ?? '-'}</p>
                <p>IP: {entry.ip_address ?? '-'}</p>
                <p>{new Date(entry.created_at).toLocaleString('id-ID')}</p>
              </div>
            </div>

            <div class="mt-5 rounded-3xl border border-ink-100 bg-white p-4">
              <p class="text-xs font-semibold uppercase tracking-[0.24em] text-ink-300">
                Payload Masked
              </p>
              <pre class="mt-3 overflow-x-auto text-xs leading-6 text-ink-700">{formatPayload(entry)}</pre>
            </div>
          </article>
        {/each}
      {/if}
    </section>
  </div>
{/if}
