<script lang="ts">
  import '../app.css';
  import { onMount } from 'svelte';
  import { navigating } from '$app/stores';
  import { ensureThemeHydrated } from '$lib/theme';

  onMount(() => {
    ensureThemeHydrated();
  });
</script>

<div class="relative min-h-screen">
  <div
    class:opacity-100={$navigating}
    class="pointer-events-none fixed inset-x-0 top-0 z-50 h-1.5 overflow-hidden bg-transparent opacity-0 transition-opacity duration-150"
  >
    <div class="h-full w-1/3 animate-[loading-bar_1.1s_ease-in-out_infinite] rounded-full bg-linear-to-r from-brand-300 via-accent-300 to-brand-500 shadow-[0_0_18px_rgba(34,201,119,0.45)]"></div>
  </div>

  <slot />
</div>
