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

<div class="toolbar-panel">
  <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
    <div class="space-y-1 text-sm text-ink-700">
      <p class="font-semibold text-ink-900">Pagination</p>
      <p>
        Menampilkan {formatNumber(startIndex)}-{formatNumber(endIndex)} dari {formatNumber(totalItems)} row.
      </p>
    </div>

    <div class="toolbar-actions">
      <label class="field-stack min-w-0 text-sm text-ink-700">
        <span class="field-label">Per page</span>
        <select
          bind:value={pageSize}
          class="field-select min-w-[6.5rem] px-3 py-2 text-sm"
        >
          {#each pageSizeOptions as option}
            <option value={option}>{option}</option>
          {/each}
        </select>
      </label>

      <div class="flex flex-wrap items-center gap-2">
        <Button variant="outline" size="sm" onclick={() => (page = Math.max(1, page - 1))} disabled={page <= 1}>
          Prev
        </Button>
        <span class="min-w-[7rem] flex-1 text-center text-sm font-medium text-ink-700 sm:flex-none">
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
</div>
