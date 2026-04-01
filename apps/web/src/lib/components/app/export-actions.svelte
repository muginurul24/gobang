<script lang="ts">
  import Button from '$lib/components/ui/button/button.svelte';
  import { formatNumber } from '$lib/formatters';

  export let count = 0;
  export let disabled = false;
  export let onCsv: (() => void) | undefined = undefined;
  export let onXlsx: (() => void) | undefined = undefined;
  export let onPdf: (() => void) | undefined = undefined;
</script>

<div class="rounded-[1.6rem] border border-ink-100 bg-white/82 px-4 py-4 shadow-[0_16px_34px_rgba(7,16,12,0.08)]">
  <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
    <div>
      <p class="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-ink-300">
        Export
      </p>
      <p class="mt-1 text-sm text-ink-700">
        {formatNumber(count)} row sesuai filter aktif siap diunduh.
      </p>
    </div>

    <div class="flex flex-wrap gap-2">
      <Button variant="outline" size="sm" onclick={() => onCsv?.()} disabled={disabled}>
        CSV
      </Button>
      <Button variant="outline" size="sm" onclick={() => onXlsx?.()} disabled={disabled}>
        Excel
      </Button>
      <Button variant="brand" size="sm" onclick={() => onPdf?.()} disabled={disabled}>
        PDF
      </Button>
    </div>
  </div>
</div>
