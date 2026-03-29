<script lang="ts">
  const cards = [
    { title: 'Stores', value: '0', note: 'Store management placeholder' },
    { title: 'Members', value: '0', note: 'Member mapping akan masuk setelah hari 15' },
    { title: 'Balance', value: 'Rp0', note: 'Ledger dashboard masuk setelah hari 13-14' }
  ];
</script>

<svelte:head>
  <title>App | onixggr</title>
</svelte:head>

<section class="space-y-6">
  <div class="glass-panel rounded-[2rem] px-6 py-7">
    <p class="text-xs font-semibold uppercase tracking-[0.24em] text-accent-700">
      Dashboard Placeholder
    </p>
    <h2 class="mt-3 font-display text-4xl font-bold tracking-tight text-ink-900">
      Shell aplikasi internal sudah siap.
    </h2>
    <p class="mt-3 max-w-3xl text-sm leading-7 text-ink-700">
      Area ini akan menjadi dashboard utama sesudah auth, store, ledger, dan notification
      selesai. Untuk sekarang fokusnya adalah memastikan struktur route dan layout final sudah
      tidak berubah-ubah lagi.
    </p>
  </div>

  <div class="grid gap-4 md:grid-cols-3">
    {#each cards as card}
      <article class="glass-panel rounded-[1.5rem] px-5 py-5">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
          {card.title}
        </p>
        <p class="mt-3 font-display text-4xl font-bold text-ink-900">{card.value}</p>
        <p class="mt-2 text-sm leading-6 text-ink-700">{card.note}</p>
      </article>
    {/each}
  </div>
</section>
