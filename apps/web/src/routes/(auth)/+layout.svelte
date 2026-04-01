<script lang="ts">
  import ThemeToggle from '$lib/components/app/theme-toggle.svelte';
</script>

<div class="matrix-page flex min-h-screen items-center justify-center px-4 py-8">
  <div class="shell-width space-y-4">
    <div class="flex justify-end">
      <ThemeToggle compact={true} />
    </div>

    <div class="grid items-stretch gap-6 xl:grid-cols-[0.88fr_1.12fr]">
    <section class="surface-dark surface-grid hidden rounded-[2.6rem] p-8 text-white xl:flex xl:flex-col xl:justify-between">
      <div>
        <p class="status-chip w-fit">Secure Access</p>
        <h1 class="mt-6 font-display text-5xl font-bold tracking-tight">
          High-trust dashboard untuk operasi toko, provider, dan payment flow.
        </h1>
        <p class="mt-5 max-w-xl text-base leading-8 text-white/70">
          Session dashboard sudah memakai refresh cookie HttpOnly, CSRF protection, one-session
          policy, TOTP optional, recovery code, dan IP allowlist.
        </p>
      </div>

      <ul class="command-list text-sm leading-6 text-white/72">
        <li>Owner dan karyawan dibatasi tenant scope sesuai store relation.</li>
        <li>Store API memakai token terpisah dari dashboard login browser.</li>
        <li>Security center, audit, dan notification feed siap dipakai dari UI.</li>
      </ul>
    </section>

    <div class="glass-panel mx-auto w-full max-w-xl rounded-[2.4rem] p-6 md:p-8 xl:mx-0">
      <slot />
    </div>
    </div>
  </div>
</div>
