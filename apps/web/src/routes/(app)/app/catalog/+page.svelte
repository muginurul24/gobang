<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import { authSession, hydrateAuthSession } from '$lib/auth/client';
  import {
    fetchCatalogGames,
    fetchCatalogProviders,
    type CatalogGame,
    type CatalogProvider
  } from '$lib/provider-catalog/client';

  let loading = true;
  let providers: CatalogProvider[] = [];
  let games: CatalogGame[] = [];
  let providerQuery = '';
  let gameQuery = '';
  let statusFilter = '';
  let selectedProvider = '';
  let errorMessage = '';

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

    const providersResponse = await fetchCatalogProviders({
      query: providerQuery,
      status: statusFilter,
      limit: 30
    });

    if (!(await ensureAuthorized(providersResponse.message))) {
      return;
    }

    if (!providersResponse.status || providersResponse.message !== 'SUCCESS') {
      errorMessage = toMessage(providersResponse.message);
      loading = false;
      return;
    }

    providers = providersResponse.data ?? [];
    if (selectedProvider && !providers.some((provider) => provider.provider_code === selectedProvider)) {
      selectedProvider = '';
    }
    if (!selectedProvider && providers.length > 0) {
      selectedProvider = providers[0].provider_code;
    }

    const gamesResponse = await fetchCatalogGames({
      provider_code: selectedProvider,
      query: gameQuery,
      status: statusFilter,
      limit: 120
    });

    if (!(await ensureAuthorized(gamesResponse.message))) {
      return;
    }

    if (!gamesResponse.status || gamesResponse.message !== 'SUCCESS') {
      errorMessage = toMessage(gamesResponse.message);
      loading = false;
      return;
    }

    games = gamesResponse.data ?? [];
    loading = false;
  }

  async function ensureAuthorized(message: string) {
    if (message === 'UNAUTHORIZED') {
      await goto('/login');
      return false;
    }

    return true;
  }

  function toMessage(message: string) {
    switch (message) {
      case 'INTERNAL_ERROR':
        return 'Katalog provider lokal belum bisa dibaca.';
      default:
        return message.replaceAll('_', ' ');
    }
  }

  function statusLabel(status: number) {
    return status === 1 ? 'Open' : 'Maintenance';
  }
</script>

<div class="space-y-6">
  <section class="glass-panel rounded-[2rem] px-6 py-6">
    <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">Provider Catalog</p>
    <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
      Browse provider dan game dari catalog lokal
    </h2>
    <p class="mt-3 max-w-3xl text-sm leading-6 text-ink-700">
      Halaman ini membaca data hasil sync lokal dari PostgreSQL. Launch game memakai catalog ini
      untuk validasi provider dan game code sebelum request diteruskan ke NexusGGR.
    </p>
  </section>

  <section class="grid gap-6 xl:grid-cols-[360px_minmax(0,1fr)]">
    <div class="glass-panel rounded-[2rem] px-5 py-5">
      <div class="grid gap-3">
        <label class="text-sm font-medium text-ink-800" for="provider-query">Cari provider</label>
        <input
          id="provider-query"
          bind:value={providerQuery}
          class="rounded-2xl border border-ink-100 bg-canvas-50 px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-brand-400"
          placeholder="PRAGMATIC atau nama provider"
        />

        <label class="text-sm font-medium text-ink-800" for="status-filter">Status</label>
        <select
          id="status-filter"
          bind:value={statusFilter}
          class="rounded-2xl border border-ink-100 bg-canvas-50 px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-brand-400"
        >
          <option value="">Semua</option>
          <option value="1">Open</option>
          <option value="0">Maintenance</option>
        </select>

        <button
          class="rounded-2xl bg-brand-600 px-4 py-3 text-sm font-semibold text-white transition hover:bg-brand-700"
          disabled={loading}
          on:click={loadScreen}
        >
          Terapkan Filter
        </button>
      </div>

      <div class="mt-5 space-y-3">
        {#if loading}
          <p class="text-sm text-ink-700">Memuat catalog provider...</p>
        {:else if providers.length === 0}
          <p class="text-sm text-ink-700">Belum ada provider di catalog lokal.</p>
        {:else}
          {#each providers as provider}
            <button
              class={`w-full rounded-[1.4rem] border px-4 py-4 text-left transition ${
                selectedProvider === provider.provider_code
                  ? 'border-brand-300 bg-brand-50'
                  : 'border-ink-100 bg-white hover:border-brand-200 hover:bg-canvas-50'
              }`}
              on:click={async () => {
                selectedProvider = provider.provider_code;
                await loadScreen();
              }}
            >
              <div class="flex items-start justify-between gap-3">
                <div>
                  <p class="text-sm font-semibold text-ink-900">{provider.provider_code}</p>
                  <p class="mt-1 text-sm text-ink-700">{provider.provider_name}</p>
                </div>
                <span class="rounded-full bg-canvas-100 px-3 py-1 text-xs font-semibold text-ink-700">
                  {statusLabel(provider.status)}
                </span>
              </div>
            </button>
          {/each}
        {/if}
      </div>
    </div>

    <div class="glass-panel rounded-[2rem] px-5 py-5">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">Games</p>
          <h3 class="mt-2 font-display text-2xl font-bold tracking-tight text-ink-900">
            {selectedProvider || 'Pilih provider'}
          </h3>
        </div>

        <div class="w-full max-w-sm">
          <label class="text-sm font-medium text-ink-800" for="game-query">Cari game</label>
          <input
            id="game-query"
            bind:value={gameQuery}
            class="mt-2 w-full rounded-2xl border border-ink-100 bg-canvas-50 px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-brand-400"
            placeholder="game_code atau nama game"
            on:keydown={async (event) => {
              if (event.key === 'Enter') {
                await loadScreen();
              }
            }}
          />
        </div>
      </div>

      {#if errorMessage}
        <div class="mt-4 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {errorMessage}
        </div>
      {/if}

      <div class="mt-5 overflow-hidden rounded-[1.6rem] border border-ink-100">
        <table class="min-w-full divide-y divide-ink-100 bg-white text-left text-sm">
          <thead class="bg-canvas-50 text-ink-700">
            <tr>
              <th class="px-4 py-3 font-semibold">Game Code</th>
              <th class="px-4 py-3 font-semibold">Game Name</th>
              <th class="px-4 py-3 font-semibold">Status</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-ink-100 text-ink-800">
            {#if loading}
              <tr>
                <td class="px-4 py-5 text-ink-700" colspan="3">Memuat game catalog...</td>
              </tr>
            {:else if games.length === 0}
              <tr>
                <td class="px-4 py-5 text-ink-700" colspan="3">
                  Tidak ada game yang cocok dengan filter saat ini.
                </td>
              </tr>
            {:else}
              {#each games as game}
                <tr>
                  <td class="px-4 py-4 font-mono text-xs text-ink-900">{game.game_code}</td>
                  <td class="px-4 py-4">{game.game_name || '-'}</td>
                  <td class="px-4 py-4">{statusLabel(game.status)}</td>
                </tr>
              {/each}
            {/if}
          </tbody>
        </table>
      </div>
    </div>
  </section>
</div>
