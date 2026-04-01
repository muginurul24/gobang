<script lang="ts">
  import Button from '$lib/components/ui/button/button.svelte';
  import ThemeToggle from '$lib/components/app/theme-toggle.svelte';
</script>

<div class="matrix-page relative min-h-screen overflow-hidden">
  <div class="pointer-events-none absolute inset-0 hero-grid opacity-20"></div>
  <div class="pointer-events-none absolute inset-x-0 top-0 h-[32rem] bg-linear-to-b from-brand-500/10 via-accent-300/6 to-transparent blur-3xl"></div>

  <header class="relative z-10">
    <div class="shell-width mx-auto flex items-center justify-between py-5">
      <a href="/" class="flex items-center gap-3">
        <span class="grid h-11 w-11 place-items-center rounded-2xl border border-white/12 bg-white/6 font-mono text-sm font-semibold text-brand-300">
          ON
        </span>
        <span>
          <span class="block font-display text-xl font-bold tracking-tight text-white">onixggr</span>
          <span class="block text-[0.68rem] uppercase tracking-[0.28em] text-white/50">
            matrix commerce bridge
          </span>
        </span>
      </a>

      <nav class="flex items-center gap-3 text-sm text-white/72">
        <a class="transition hover:text-white" href="/">Home</a>
        <a class="transition hover:text-white" href="/login">Login</a>
        <ThemeToggle compact={true} />
        <Button
          variant="outline"
          size="sm"
          class="border-white/18 bg-white/8 text-white hover:bg-white/12"
          onclick={() => (window.location.href = '/app')}
        >
          Open App
        </Button>
      </nav>
    </div>
  </header>

  <slot />
</div>
