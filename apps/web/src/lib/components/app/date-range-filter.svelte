<script lang="ts">
  export let label = 'Rentang waktu';
  export let start = '';
  export let end = '';

  function applyPreset(days: number) {
    const endDate = new Date();
    const startDate = new Date(endDate.getTime() - days * 24 * 60 * 60 * 1000);
    start = toLocalInputValue(startDate);
    end = toLocalInputValue(endDate);
  }

  function clearRange() {
    start = '';
    end = '';
  }

  function toLocalInputValue(value: Date) {
    const offset = value.getTimezoneOffset() * 60 * 1000;
    const local = new Date(value.getTime() - offset);
    return local.toISOString().slice(0, 16);
  }
</script>

<div class="date-filter-shell">
  <div class="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
    <div class="space-y-1">
      <p class="section-kicker !text-brand-700">Time Filter</p>
      <p class="text-sm font-semibold text-ink-900">{label}</p>
      <p class="field-help">
        Pakai preset cepat atau pilih rentang khusus. Semua halaman membaca filter ini tanpa memaksa layout melebar.
      </p>
    </div>

    <div class="date-filter-presets">
      <button class="date-filter-preset" type="button" onclick={() => applyPreset(1)}>
        24h
      </button>
      <button class="date-filter-preset" type="button" onclick={() => applyPreset(7)}>
        7d
      </button>
      <button class="date-filter-preset" type="button" onclick={() => applyPreset(30)}>
        30d
      </button>
      <button class="date-filter-preset" type="button" onclick={clearRange}>
        Clear
      </button>
    </div>
  </div>

  <div class="date-filter-grid mt-4">
    <label class="field-stack date-filter-field">
      <span class="field-label">{label} · Dari</span>
      <span class="date-filter-stamp">start</span>
      <input bind:value={start} class="field-input" type="datetime-local" />
    </label>

    <label class="field-stack date-filter-field">
      <span class="field-label">{label} · Sampai</span>
      <span class="date-filter-stamp">end</span>
      <input bind:value={end} class="field-input" type="datetime-local" />
    </label>
  </div>
</div>
