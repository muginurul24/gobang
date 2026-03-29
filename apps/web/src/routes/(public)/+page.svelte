<script lang="ts">
  import Button from '$lib/components/ui/button/button.svelte';

  const pillars = [
    'Go API, worker, dan scheduler dalam satu repo',
    'PostgreSQL sebagai source of truth, Redis sebagai akselerator',
    'Dashboard SvelteKit dengan shell terpisah untuk public, auth, dan app'
  ];
</script>

<svelte:head>
  <title>onixggr</title>
  <meta
    name="description"
    content="Landing placeholder for the onixggr multi-tenant API bridge platform."
  />
</svelte:head>

<main class="relative z-10 shell-width mx-auto pb-16 pt-8">
  <section class="glass-panel overflow-hidden rounded-[2rem] border border-white/70">
    <div class="grid gap-10 px-6 py-10 md:grid-cols-[1.2fr_0.8fr] md:px-10 md:py-14">
      <div class="space-y-6">
        <p class="inline-flex rounded-full border border-brand-300/70 bg-brand-100 px-3 py-1 text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
          Public Shell
        </p>

        <div class="space-y-4">
          <h1 class="font-display text-5xl font-bold tracking-tight text-ink-900 md:text-7xl">
            API bridge multi-tenant dengan fondasi yang sudah terkunci dari awal.
          </h1>
          <p class="max-w-2xl text-lg leading-8 text-ink-700">
            Halaman ini menandai shell public final untuk fase awal. Layout auth dan app sudah
            dipisah, loading bar navigasi sudah aktif, dan placeholder dashboard siap diisi domain
            berikutnya.
          </p>
        </div>

        <div class="flex flex-wrap gap-3">
          <Button variant="brand" size="lg" onclick={() => (window.location.href = '/login')}>
            Masuk Dashboard
          </Button>
          <Button variant="outline" size="lg" onclick={() => (window.location.href = '/app')}>
            Lihat App Shell
          </Button>
        </div>
      </div>

      <div class="rounded-[1.5rem] bg-ink-900 p-6 text-white">
        <p class="text-xs font-semibold uppercase tracking-[0.28em] text-brand-300">
          Foundation Snapshot
        </p>
        <ul class="mt-5 space-y-4">
          {#each pillars as pillar}
            <li class="rounded-2xl border border-white/10 bg-white/5 px-4 py-4 text-sm leading-6 text-ink-50">
              {pillar}
            </li>
          {/each}
        </ul>
      </div>
    </div>
  </section>
</main>
