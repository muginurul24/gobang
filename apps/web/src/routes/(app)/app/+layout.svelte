<script lang="ts">
  const nav = [
    { href: '/app', label: 'Dashboard' },
    { href: '/app/chat', label: 'Global Chat' },
    { href: '/', label: 'Back to Public' }
  ];
</script>

<div class="shell-width mx-auto flex min-h-screen flex-col gap-6 py-6 lg:flex-row">
  <aside class="glass-panel w-full rounded-[2rem] p-5 lg:sticky lg:top-6 lg:h-[calc(100vh-3rem)] lg:w-80">
    <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">App Shell</p>
    <h1 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">onixggr</h1>
    <p class="mt-3 text-sm leading-6 text-ink-700">
      Placeholder layout dashboard untuk owner, karyawan, dan operator internal.
    </p>

    <nav class="mt-8 space-y-2">
      {#each nav as item}
        <a
          class="block rounded-2xl border border-transparent px-4 py-3 text-sm font-medium text-ink-700 transition hover:border-ink-100 hover:bg-canvas-100 hover:text-ink-900"
          href={item.href}
        >
          {item.label}
        </a>
      {/each}
    </nav>
  </aside>

  <main class="min-w-0 flex-1">
    <slot />
  </main>
</div>
