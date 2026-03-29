<script lang="ts">
  import Button from '$lib/components/ui/button/button.svelte';
</script>

<div class="relative min-h-screen overflow-hidden">
  <div class="pointer-events-none absolute inset-0 hero-grid opacity-40"></div>

  <header class="relative z-10">
    <div class="shell-width mx-auto flex items-center justify-between py-6">
      <a href="/" class="font-display text-xl font-bold tracking-tight text-ink-900">onixggr</a>

      <nav class="flex items-center gap-3 text-sm text-ink-700">
        <a class="hover:text-ink-900" href="/">Home</a>
        <a class="hover:text-ink-900" href="/login">Login</a>
        <Button variant="outline" size="sm" class="bg-white/70" onclick={() => (window.location.href = '/app')}>
          Open App
        </Button>
      </nav>
    </div>
  </header>

  <slot />
</div>
