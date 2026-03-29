<script lang="ts">
  import Button from '$lib/components/ui/button/button.svelte';
</script>

<svelte:head>
  <title>Login | onixggr</title>
</svelte:head>

<div class="space-y-6">
  <div class="space-y-2">
    <p class="text-xs font-semibold uppercase tracking-[0.24em] text-accent-700">Auth Shell</p>
    <h1 class="font-display text-3xl font-bold tracking-tight text-ink-900">Masuk ke dashboard</h1>
    <p class="text-sm leading-6 text-ink-700">
      Placeholder login untuk fase auth. Form ini sudah memakai layout auth final dan siap
      dihubungkan ke endpoint login pada hari berikutnya.
    </p>
  </div>

  <form class="space-y-4">
    <label class="block space-y-2">
      <span class="text-sm font-medium text-ink-700">Email atau username</span>
      <input
        class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
        type="text"
        placeholder="owner-demo"
      />
    </label>

    <label class="block space-y-2">
      <span class="text-sm font-medium text-ink-700">Password</span>
      <input
        class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
        type="password"
        placeholder="••••••••"
      />
    </label>

    <Button variant="brand" size="lg" class="w-full">
      Login Placeholder
    </Button>
  </form>
</div>
