<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import { authSession, initializeAuthSession } from '$lib/auth/client';
  import EmptyState from '$lib/components/app/empty-state.svelte';
  import MetricCard from '$lib/components/app/metric-card.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import PaginationControls from '$lib/components/app/pagination-controls.svelte';
  import Button from '$lib/components/ui/button/button.svelte';
  import { formatDateTime, formatNumber } from '$lib/formatters';
  import {
    fetchCatalogGames,
    fetchCatalogProviders,
    type CatalogGame,
    type CatalogProvider,
  } from '$lib/provider-catalog/client';

  let loading = true;
  let providers: CatalogProvider[] = [];
  let games: CatalogGame[] = [];
  let providerTotalCount = 0;
  let gameTotalCount = 0;
  let providerQuery = '';
  let gameQuery = '';
  let statusFilter = '';
  let selectedProvider = '';
  let errorMessage = '';
  let providerPage = 1;
  let providerPageSize = 8;
  let gamePage = 1;
  let gamePageSize = 12;
  let lastProviderQueryKey = '';
  let lastGameQueryKey = '';

  $: openProviderCount = providers.filter((provider) => provider.status === 1).length;
  $: maintenanceProviderCount = providers.filter((provider) => provider.status !== 1).length;
  $: openGameCount = games.filter((game) => game.status === 1).length;
  $: maintenanceGameCount = games.filter((game) => game.status !== 1).length;
  $: if (!loading) {
    const nextProviderKey = `${providerPage}:${providerPageSize}`;
    if (nextProviderKey !== lastProviderQueryKey) {
      void loadProviders();
    }
  }
  $: if (!loading && selectedProvider !== '') {
    const nextGameKey = `${selectedProvider}:${gamePage}:${gamePageSize}`;
    if (nextGameKey !== lastGameQueryKey) {
      void loadGames();
    }
  }

  onMount(async () => {
    await initializeAuthSession();

    if (!$authSession) {
      await goto('/login');
      return;
    }

    await loadProviders();
    loading = false;
  });

  async function loadProviders() {
    errorMessage = '';

    const providersResponse = await fetchCatalogProviders({
      query: providerQuery,
      status: statusFilter,
      limit: providerPageSize,
      offset: (providerPage - 1) * providerPageSize,
    });

    if (!(await ensureAuthorized(providersResponse.message))) {
      return;
    }

    if (!providersResponse.status || providersResponse.message !== 'SUCCESS') {
      errorMessage = toMessage(providersResponse.message);
      return;
    }

    providers = providersResponse.data.items ?? [];
    providerTotalCount = providersResponse.data.total_count ?? 0;
    lastProviderQueryKey = `${providerPage}:${providerPageSize}`;

    const previousProvider = selectedProvider;
    if (
      selectedProvider &&
      !providers.some((provider) => provider.provider_code === selectedProvider)
    ) {
      selectedProvider = '';
    }
    if (!selectedProvider && providers.length > 0) {
      selectedProvider = providers[0].provider_code;
    }

    if (selectedProvider === '') {
      games = [];
      gameTotalCount = 0;
      lastGameQueryKey = '';
      return;
    }

    if (selectedProvider !== previousProvider) {
      gamePage = 1;
      lastGameQueryKey = '';
    }

    await loadGames();
  }

  async function loadGames() {
    if (selectedProvider === '') {
      games = [];
      gameTotalCount = 0;
      lastGameQueryKey = '';
      return;
    }

    const gamesResponse = await fetchCatalogGames({
      provider_code: selectedProvider,
      query: gameQuery,
      status: statusFilter,
      limit: gamePageSize,
      offset: (gamePage - 1) * gamePageSize,
    });

    if (!(await ensureAuthorized(gamesResponse.message))) {
      return;
    }

    if (!gamesResponse.status || gamesResponse.message !== 'SUCCESS') {
      errorMessage = toMessage(gamesResponse.message);
      return;
    }

    games = gamesResponse.data.items ?? [];
    gameTotalCount = gamesResponse.data.total_count ?? 0;
    lastGameQueryKey = `${selectedProvider}:${gamePage}:${gamePageSize}`;
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

  async function applyProviderFilters() {
    providerPage = 1;
    await loadProviders();
  }

  async function resetProviderFilters() {
    providerQuery = '';
    statusFilter = '';
    providerPage = 1;
    gamePage = 1;
    await loadProviders();
  }

  async function applyGameFilters() {
    gamePage = 1;
    await loadGames();
  }
</script>

<svelte:head>
  <title>Catalog | onixggr</title>
</svelte:head>

<div class="space-y-6">
  <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
    <div class="grid gap-6 xl:grid-cols-[1.12fr_0.88fr]">
      <div class="space-y-4">
        <div class="flex flex-wrap gap-3">
          <span class="status-chip">provider sync</span>
          <span class="status-chip">{formatNumber(providerTotalCount)} provider</span>
          <span class="status-chip">{formatNumber(gameTotalCount)} game</span>
        </div>
        <div class="space-y-3">
          <p class="section-kicker">Provider catalog</p>
          <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
            Browse provider dan game yang sudah tersinkron dari upstream.
          </h1>
          <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
            Catalog lokal dipakai backend untuk validasi launch game dan integrasi store API.
            Frontend membaca hasil sync PostgreSQL, bukan langsung memukul upstream.
          </p>
        </div>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <MetricCard
          class="h-full"
          eyebrow="Provider"
          title="Open providers"
          value={formatNumber(openProviderCount)}
          detail="Provider terbuka pada page hasil backend saat ini."
          tone="brand"
        />
        <MetricCard
          class="h-full"
          eyebrow="Game"
          title="Open games"
          value={formatNumber(openGameCount)}
          detail="Game terbuka pada page hasil backend saat ini."
          tone="accent"
        />
      </div>
    </div>
  </section>

  {#if errorMessage}
    <Notice tone="error" title="Catalog Error" message={errorMessage} />
  {/if}

  <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          eyebrow="Provider"
          title="Maintenance"
          value={formatNumber(maintenanceProviderCount)}
          detail="Provider maintenance pada page hasil backend saat ini."
        />
        <MetricCard
          eyebrow="Game"
          title="Maintenance games"
          value={formatNumber(maintenanceGameCount)}
          detail="Game maintenance pada page hasil backend saat ini."
        />
    <MetricCard
      eyebrow="Selected"
      title="Provider focus"
      value={selectedProvider || '-'}
      detail="Provider aktif yang dipakai untuk query game saat ini."
      tone="accent"
    />
    <MetricCard
      eyebrow="Sync"
      title="Role"
      value={$authSession?.user.role ?? '-'}
      detail="Catalog dapat dibaca dari dashboard untuk validasi operasional dan integrasi."
      tone="brand"
    />
  </div>

  <section class="grid gap-6 xl:grid-cols-[minmax(17rem,20rem)_minmax(0,1fr)]">
    <div class="glass-panel rounded-[2.2rem] px-5 py-5">
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

        <div class="flex flex-wrap gap-3">
          <Button variant="brand" size="lg" onclick={applyProviderFilters} disabled={loading}>
            Terapkan Filter
          </Button>
          <Button variant="outline" size="lg" onclick={resetProviderFilters} disabled={loading}>
            Reset
          </Button>
        </div>
      </div>

      <div class="mt-5 space-y-3">
        {#if loading}
          <p class="text-sm text-ink-700">Memuat catalog provider...</p>
        {:else if providers.length === 0}
          <p class="text-sm text-ink-700">Belum ada provider di catalog lokal.</p>
        {:else}
          {#each providers as provider}
            <button
              class={`w-full rounded-[1.5rem] border px-4 py-4 text-left transition ${
                selectedProvider === provider.provider_code
                  ? 'border-brand-300 bg-brand-100/40'
                  : 'border-ink-100 bg-white hover:border-brand-200 hover:bg-canvas-50'
              }`}
              on:click={async () => {
                selectedProvider = provider.provider_code;
                gamePage = 1;
                await loadGames();
              }}
            >
              <div class="flex items-start justify-between gap-3">
                <div>
                  <p class="text-sm font-semibold text-ink-900">{provider.provider_code}</p>
                  <p class="mt-1 text-sm text-ink-700">{provider.provider_name}</p>
                  <p class="mt-2 text-xs text-ink-500">
                    Synced {formatDateTime(provider.synced_at)}
                  </p>
                </div>
                <span class="surface-chip">{statusLabel(provider.status)}</span>
              </div>
            </button>
          {/each}

          {#if providerTotalCount > 0}
            <div class="pt-2">
              <PaginationControls
                bind:page={providerPage}
                bind:pageSize={providerPageSize}
                totalItems={providerTotalCount}
              />
            </div>
          {/if}
        {/if}
      </div>
    </div>

    <div class="glass-panel rounded-[2.2rem] px-5 py-5">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p class="section-kicker !text-brand-700">Games</p>
          <h2 class="mt-2 font-display text-3xl font-bold tracking-tight text-ink-900">
            {selectedProvider || 'Pilih provider'}
          </h2>
          <p class="mt-2 text-sm leading-6 text-ink-700">
            Provider code dan game code ini yang dipakai backend untuk validasi launch flow.
            Search dan pagination game juga sekarang dieksekusi di backend.
          </p>
        </div>

        <div class="w-full max-w-sm space-y-3">
          <label class="text-sm font-medium text-ink-800" for="game-query">Cari game</label>
          <input
            id="game-query"
            bind:value={gameQuery}
            class="mt-2 w-full rounded-2xl border border-ink-100 bg-canvas-50 px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-brand-400"
            placeholder="game_code atau nama game"
            on:keydown={async (event) => {
              if (event.key === 'Enter') {
                await applyGameFilters();
              }
            }}
          />

          <Button variant="outline" size="lg" onclick={applyGameFilters} disabled={loading}>
            Refresh Games
          </Button>
        </div>
      </div>

      {#if loading}
        <div class="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {#each Array(6) as _}
            <article class="animate-pulse rounded-[1.7rem] border border-ink-100 bg-canvas-50 px-4 py-4">
              <div class="h-4 w-28 rounded-full bg-white"></div>
              <div class="mt-3 h-3 w-full rounded-full bg-white"></div>
              <div class="mt-2 h-3 w-2/3 rounded-full bg-white"></div>
            </article>
          {/each}
        </div>
      {:else if games.length === 0}
        <div class="mt-6">
          <EmptyState
            eyebrow="Game Catalog"
            title="Tidak ada game yang cocok"
            body="Coba ganti provider, query, atau filter status untuk melihat katalog game lain."
          />
        </div>
      {:else}
        <div class="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {#each games as game}
            <article class="overflow-hidden rounded-[1.8rem] border border-ink-100 bg-white shadow-[0_16px_34px_rgba(7,16,12,0.08)]">
              {#if game.banner_url !== ''}
                <div class="h-36 w-full overflow-hidden bg-canvas-100">
                  <img
                    alt={game.game_name}
                    class="h-full w-full object-cover"
                    loading="lazy"
                    src={game.banner_url}
                  />
                </div>
              {:else}
                <div class="surface-dark surface-grid flex h-36 items-end px-4 py-4 text-white">
                  <p class="text-[0.72rem] font-semibold uppercase tracking-[0.24em]">
                    {game.provider_code}
                  </p>
                </div>
              {/if}

              <div class="p-4">
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <p class="text-sm font-semibold text-ink-900">{game.game_name || '-'}</p>
                    <p class="mt-2 font-mono text-xs text-ink-500">{game.game_code}</p>
                  </div>
                  <span class="surface-chip">{statusLabel(game.status)}</span>
                </div>
                <p class="mt-4 text-xs text-ink-500">
                  Synced {formatDateTime(game.synced_at)}
                </p>
              </div>
            </article>
          {/each}
        </div>

        {#if gameTotalCount > 0}
          <div class="mt-6">
            <PaginationControls bind:page={gamePage} bind:pageSize={gamePageSize} totalItems={gameTotalCount} />
          </div>
        {/if}
      {/if}
    </div>
  </section>
</div>
