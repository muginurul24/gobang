<script lang="ts">
  import Button from '$lib/components/ui/button/button.svelte';

  const pillars = [
    'Store API tokenized untuk member, deposit, withdraw, balance, launch, dan QRIS payment.',
    'Ledger, reconcile worker, webhook finalizer, dan callback retry untuk flow uang yang aman.',
    'Realtime dashboard, notification feed, observability, dan audit viewer untuk operasi harian.'
  ];

  const highlights = [
    'QRIS store topup dan member payment',
    'Game deposit dan withdraw berbasis store token',
    'Withdraw dashboard inquiry -> reserve -> transfer -> finalize',
    'Notification stream, chat, audit, dan security center'
  ];
</script>

<svelte:head>
  <title>onixggr</title>
  <meta
    name="description"
    content="Landing placeholder for the onixggr multi-tenant API bridge platform."
  />
</svelte:head>

<main class="relative z-10 shell-width mx-auto pb-20 pt-6" id="app-main">
  <section class="surface-dark surface-grid overflow-hidden rounded-[2.6rem]">
    <div class="grid gap-10 px-6 py-8 md:px-10 md:py-10 xl:grid-cols-[1.15fr_0.85fr]">
      <div class="space-y-6 text-white">
        <p class="status-chip w-fit">
          Public Shell
        </p>

        <div class="space-y-4">
          <h1 class="font-display text-5xl font-bold tracking-tight md:text-7xl">
            Commerce command
            <span class="brand-gradient-text">matrix</span>
            untuk operasi toko multi-tenant.
          </h1>
          <p class="max-w-3xl text-base leading-8 text-white/70 md:text-lg">
            onixggr menyatukan store API, dashboard operasional, worker reconcile, audit, dan
            observability dalam satu command surface yang siap dipasang di domain toko Anda.
          </p>
        </div>

        <div class="flex flex-wrap gap-3">
          <Button variant="brand" size="lg" onclick={() => (window.location.href = '/login')}>
            Masuk Dashboard
          </Button>
          <Button
            variant="outline"
            size="lg"
            class="border-white/16 bg-white/8 text-white hover:bg-white/12"
            onclick={() => (window.location.href = '/app')}
          >
            Buka Command Center
          </Button>
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          {#each highlights as highlight}
            <div class="rounded-[1.6rem] border border-white/10 bg-white/6 px-4 py-4 text-sm leading-6 text-white/78 backdrop-blur">
              {highlight}
            </div>
          {/each}
        </div>
      </div>

      <div class="grid gap-4">
        <article class="glass-panel rounded-[2rem] p-6">
          <p class="section-kicker">Architecture</p>
          <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
            Backend contract sudah matang. Frontend hanya perlu mengoperasikan.
          </h2>
          <p class="mt-3 text-sm leading-7 text-ink-700">
            Semua flow utama sudah tersedia: store management, member, QRIS, withdraw, security,
            notifications, audit, realtime, observability, dan deploy production.
          </p>
        </article>

        <article class="rounded-[2rem] border border-white/12 bg-canvas-900/86 p-6 text-white shadow-[0_22px_54px_rgba(0,0,0,0.34)]">
          <p class="text-xs font-semibold uppercase tracking-[0.28em] text-brand-300">
            Foundation Snapshot
          </p>
          <ul class="mt-5 space-y-4">
            {#each pillars as pillar}
              <li class="rounded-[1.35rem] border border-white/10 bg-white/6 px-4 py-4 text-sm leading-6 text-white/80">
                {pillar}
              </li>
            {/each}
          </ul>
        </article>
      </div>
    </div>
  </section>

  <section class="mt-6 grid gap-4 md:grid-cols-3">
    {#each [
      { label: 'Realtime', value: 'WS + Redis', body: 'owner, karyawan, dev, dan superadmin mengikuti channel sesuai scope role dan store.' },
      { label: 'Money Safety', value: 'Ledger First', body: 'reserve, commit, release, duplicate webhook guard, dan reconcile worker sudah terpasang.' },
      { label: 'Ops Surface', value: 'Audit + Metrics', body: 'notification stream, callback retry, observability, alert rules, dan staging/production checklist tersedia.' }
    ] as card}
      <article class="glass-panel rounded-[1.9rem] p-5">
        <p class="section-kicker">{card.label}</p>
        <h3 class="mt-3 font-display text-2xl font-bold tracking-tight text-ink-900">{card.value}</h3>
        <p class="mt-3 text-sm leading-6 text-ink-700">{card.body}</p>
      </article>
    {/each}
  </section>
</main>
