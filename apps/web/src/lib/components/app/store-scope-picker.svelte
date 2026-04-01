<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';

  import { formatCurrency, formatNumber } from '$lib/formatters';
  import PaginationControls from '$lib/components/app/pagination-controls.svelte';
  import Button from '$lib/components/ui/button/button.svelte';
  import {
    fetchStore,
    fetchStoreDirectory,
    isStoreLowBalance,
    type Store,
    type StoreDirectorySummary,
  } from '$lib/stores/client';

  const emptySummary: StoreDirectorySummary = {
    total_count: 0,
    active_count: 0,
    inactive_count: 0,
    banned_count: 0,
    deleted_count: 0,
    low_balance_count: 0,
  };

  export let selectedStoreID = '';
  export let selectedStore: Store | null = null;
  export let totalCount = 0;
  export let summary: StoreDirectorySummary = { ...emptySummary };
  export let title = 'Store scope';
  export let description =
    'Gunakan directory endpoint backend agar selector store tetap ringan saat data membesar.';
  export let placeholder = 'Cari store atau slug';
  export let emptyLabel = 'Tidak ada store pada page hasil query.';
  export let allowEmpty = false;
  export let allowEmptyLabel = 'Semua store dalam scope';
  export let compact = false;
  export let disabled = false;
  export let pageSize = 8;

  export let loading = true;
  export let errorMessage = '';
  let query = '';
  let page = 1;
  let pageItems: Store[] = [];
  let mounted = false;
  let lastDirectoryKey = '';
  let lastSyncedStoreID = '';

  const dispatch = createEventDispatcher<{
    change: { storeID: string; store: Store | null };
  }>();

  $: visibleItems =
    selectedStore && !pageItems.some((store) => store.id === selectedStore?.id)
      ? [selectedStore, ...pageItems]
      : pageItems;

  onMount(() => {
    mounted = true;
    void loadDirectory();

    return () => {
      mounted = false;
    };
  });

  $: if (mounted) {
    const nextDirectoryKey = `${query}:${page}:${pageSize}`;
    if (nextDirectoryKey !== lastDirectoryKey) {
      lastDirectoryKey = nextDirectoryKey;
      void loadDirectory();
    }
  }

  $: if (mounted && selectedStoreID !== lastSyncedStoreID) {
    lastSyncedStoreID = selectedStoreID;
    void syncSelectedStore(selectedStoreID);
  }

  async function loadDirectory() {
    loading = true;
    errorMessage = '';

    const response = await fetchStoreDirectory({
      query,
      limit: pageSize,
      offset: (page - 1) * pageSize,
    });
    if (!mounted) {
      return;
    }

    loading = false;

    if (!response.status || response.message !== 'SUCCESS') {
      pageItems = [];
      totalCount = 0;
      summary = { ...emptySummary };
      errorMessage =
        response.message === 'FORBIDDEN'
          ? 'Store directory tidak tersedia untuk role ini.'
          : 'Store directory belum bisa dimuat.';
      selectedStore = null;
      if (!allowEmpty) {
        selectedStoreID = '';
        dispatch('change', { storeID: '', store: null });
      }
      return;
    }

    pageItems = response.data.items ?? [];
    summary = response.data.summary ?? { ...emptySummary };
    totalCount = response.data.summary?.total_count ?? 0;

    if (selectedStoreID === '' && pageItems.length > 0 && !allowEmpty) {
      applySelection(pageItems[0].id, pageItems[0]);
      return;
    }

    if (selectedStoreID !== '') {
      const matched = pageItems.find((store) => store.id === selectedStoreID) ?? null;
      if (matched) {
        selectedStore = matched;
        dispatch('change', { storeID: matched.id, store: matched });
        return;
      }
    }

    if (pageItems.length === 0 && totalCount === 0) {
      selectedStore = null;
      if (!allowEmpty) {
        selectedStoreID = '';
        dispatch('change', { storeID: '', store: null });
      }
    }
  }

  async function syncSelectedStore(storeID: string) {
    if (!mounted) {
      return;
    }

    if (storeID.trim() === '') {
      selectedStore = null;
      return;
    }

    const matched = pageItems.find((store) => store.id === storeID) ?? null;
    if (matched) {
      selectedStore = matched;
      return;
    }

    const response = await fetchStore(storeID);
    if (!mounted) {
      return;
    }

    if (!response.status || response.message !== 'SUCCESS') {
      selectedStore = null;
      return;
    }

    selectedStore = response.data ?? null;
  }

  function applySelection(storeID: string, store: Store | null) {
    selectedStoreID = storeID;
    selectedStore = store;
    dispatch('change', { storeID, store });
  }

  async function chooseStore(storeID: string) {
    const matched = visibleItems.find((store) => store.id === storeID) ?? null;
    applySelection(storeID, matched);
    await syncSelectedStore(storeID);
  }

  async function applySearch() {
    page = 1;
    lastDirectoryKey = '';
    await loadDirectory();
  }

  async function resetSearch() {
    query = '';
    page = 1;
    lastDirectoryKey = '';
    await loadDirectory();
  }
</script>

<section class={`glass-panel ${compact ? 'rounded-[1.8rem] p-4 sm:p-5' : 'rounded-[2rem] p-5 sm:p-6'}`}>
  <div class={`flex flex-col gap-4 ${compact ? 'xl:flex-row xl:items-end xl:justify-between' : 'xl:flex-row xl:items-start xl:justify-between'}`}>
    <div class="space-y-2">
      <p class="section-kicker">Store Scope</p>
      <h3 class={`font-display font-bold tracking-tight text-ink-900 ${compact ? 'text-xl sm:text-2xl' : 'text-2xl'}`}>{title}</h3>
      <p class={`max-w-3xl leading-6 text-ink-700 ${compact ? 'text-[0.92rem]' : 'text-sm'}`}>{description}</p>
    </div>

    {#if compact}
      <div class="flex flex-wrap items-center gap-2 xl:justify-end">
        <span class="surface-chip">{formatNumber(totalCount)} visible</span>
        <span class="surface-chip">{formatNumber(summary.active_count)} active</span>
        {#if summary.low_balance_count > 0}
          <span class="status-chip">{formatNumber(summary.low_balance_count)} low balance</span>
        {/if}
        {#if selectedStore}
          <span class="surface-chip">{formatCurrency(selectedStore.current_balance)}</span>
        {:else if allowEmpty && selectedStoreID === ''}
          <span class="surface-chip">{allowEmptyLabel}</span>
        {/if}
      </div>
    {:else}
      <div class="grid gap-3 sm:grid-cols-2">
        <article class="rounded-[1.35rem] border border-white/65 bg-white/72 px-4 py-4 shadow-[0_14px_28px_rgba(7,16,12,0.08)]">
          <p class="text-[0.68rem] font-semibold uppercase tracking-[0.24em] text-ink-300">Visible</p>
          <p class="mt-2 font-display text-2xl font-semibold tracking-tight text-ink-900">{formatNumber(totalCount)}</p>
          <p class="mt-2 text-xs leading-5 text-ink-700">{formatNumber(summary.low_balance_count)} low balance in scope.</p>
        </article>
        <article class="rounded-[1.35rem] border border-white/65 bg-white/72 px-4 py-4 shadow-[0_14px_28px_rgba(7,16,12,0.08)]">
          <p class="text-[0.68rem] font-semibold uppercase tracking-[0.24em] text-ink-300">Selected</p>
          <p class="mt-2 font-semibold text-ink-900">
            {selectedStore?.name ?? (allowEmpty ? allowEmptyLabel : 'Belum ada store aktif')}
          </p>
          <p class="mt-2 text-xs leading-5 text-ink-700">
            {#if selectedStore}
              Saldo {formatCurrency(selectedStore.current_balance)}
            {:else if allowEmpty && selectedStoreID === ''}
              Filter sekarang membaca seluruh store dalam scope sesi.
            {:else}
              {emptyLabel}
            {/if}
          </p>
        </article>
      </div>
    {/if}
  </div>

  <div class={`mt-5 grid gap-4 ${compact ? 'xl:grid-cols-[minmax(0,1fr)_12rem_auto] xl:items-end' : 'xl:grid-cols-[minmax(0,1fr)_14rem_auto] xl:items-end'}`}>
    <label class="field-stack">
      <span class="field-label">Search directory</span>
      <input
        bind:value={query}
        class="field-input"
        placeholder={placeholder}
        disabled={disabled}
      />
    </label>

    <label class="field-stack">
      <span class="field-label">Page size</span>
      <select
        bind:value={pageSize}
        class="field-select"
        disabled={disabled}
      >
        <option value={6}>6</option>
        <option value={8}>8</option>
        <option value={12}>12</option>
        <option value={24}>24</option>
      </select>
    </label>

    <div class="toolbar-actions">
      <Button variant="brand" onclick={applySearch} disabled={disabled || loading}>
        Search
      </Button>
      <Button variant="outline" onclick={resetSearch} disabled={disabled || loading}>
        Reset
      </Button>
    </div>
  </div>

  <div class={`mt-4 grid gap-4 ${compact ? 'xl:grid-cols-[minmax(0,1fr)_auto] xl:items-end' : 'xl:grid-cols-[minmax(0,1fr)_auto] xl:items-end'}`}>
    <label class="field-stack">
      <span class="field-label">Store aktif</span>
      <select
        bind:value={selectedStoreID}
        class="field-select"
        disabled={disabled || loading || visibleItems.length === 0}
        onchange={(event) => chooseStore((event.currentTarget as HTMLSelectElement).value)}
      >
        {#if visibleItems.length === 0}
          <option value="">
            {#if loading}
              Memuat store...
            {:else if allowEmpty}
              {allowEmptyLabel}
            {:else}
              {emptyLabel}
            {/if}
          </option>
        {:else}
          {#if allowEmpty}
            <option value="">{allowEmptyLabel}</option>
          {/if}
          {#each visibleItems as store}
            <option value={store.id}>
              {store.name} · {store.slug} · {formatCurrency(store.current_balance)}
            </option>
          {/each}
        {/if}
      </select>
    </label>

    <div class="flex flex-wrap items-center gap-2">
      <span class="surface-chip">{formatNumber(summary.active_count)} active</span>
      <span class="surface-chip">{formatNumber(summary.inactive_count)} inactive</span>
      {#if summary.banned_count > 0}
        <span class="surface-chip">{formatNumber(summary.banned_count)} banned</span>
      {/if}
      {#if selectedStore && isStoreLowBalance(selectedStore)}
        <span class="status-chip">low balance</span>
      {/if}
    </div>
  </div>

  {#if errorMessage !== ''}
    <div class="mt-4 rounded-[1.35rem] border border-rose-200/70 bg-rose-50/88 px-4 py-3 text-sm text-rose-700">
      {errorMessage}
    </div>
  {/if}

  {#if totalCount > pageSize}
    <div class="mt-4">
      <PaginationControls bind:page pageSize={pageSize} totalItems={totalCount} />
    </div>
  {/if}
</section>
