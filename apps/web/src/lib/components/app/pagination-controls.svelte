<script lang="ts">
  import Button from '$lib/components/ui/button/button.svelte';
  import { formatNumber } from '$lib/formatters';

  export let page = 1;
  export let pageSize = 12;
  export let totalItems = 0;
  export let pageSizeOptions = [6, 12, 24, 48];

  $: totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  $: page = Math.min(Math.max(1, page), totalPages);
  $: startIndex = totalItems === 0 ? 0 : (page - 1) * pageSize + 1;
  $: endIndex = Math.min(totalItems, page * pageSize);
</script>

<div class="flex flex-col gap-3 rounded-[1.7rem] border border-ink-100 bg-canvas-50 px-4 py-4 md:flex-row md:items-center md:justify-between">
  <div class="space-y-1 text-sm text-ink-700">
    <p class="font-semibold text-ink-900">Pagination</p>
    <p>
      Menampilkan {formatNumber(startIndex)}-{formatNumber(endIndex)} dari {formatNumber(totalItems)} row.
    </p>
  </div>

  <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
    <label class="flex items-center gap-2 text-sm text-ink-700">
      <span>Per page</span>
      <select
        bind:value={pageSize}
        class="rounded-2xl border border-ink-100 bg-white px-3 py-2 text-sm text-ink-900 outline-none transition focus:border-accent-300"
      >
        {#each pageSizeOptions as option}
          <option value={option}>{option}</option>
        {/each}
      </select>
    </label>

    <div class="flex items-center gap-2">
      <Button variant="outline" size="sm" onclick={() => (page = Math.max(1, page - 1))} disabled={page <= 1}>
        Prev
      </Button>
      <span class="min-w-28 text-center text-sm font-medium text-ink-700">
        Page {formatNumber(page)} / {formatNumber(totalPages)}
      </span>
      <Button
        variant="outline"
        size="sm"
        onclick={() => (page = Math.min(totalPages, page + 1))}
        disabled={page >= totalPages}
      >
        Next
      </Button>
    </div>
  </div>
</div>
