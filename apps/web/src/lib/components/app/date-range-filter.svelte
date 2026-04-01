<script lang="ts">
  export let label = 'Rentang waktu';
  export let start = '';
  export let end = '';
</script>

<div class="grid gap-3 sm:grid-cols-2">
  <label class="space-y-2">
    <span class="text-sm font-medium text-ink-700">{label} · Dari</span>
    <input
      bind:value={start}
      class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
      type="datetime-local"
    />
  </label>

  <label class="space-y-2">
    <span class="text-sm font-medium text-ink-700">{label} · Sampai</span>
    <input
      bind:value={end}
      class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
      type="datetime-local"
    />
  </label>
</div>
