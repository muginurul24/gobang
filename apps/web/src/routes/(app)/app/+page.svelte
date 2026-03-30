<script lang="ts">
  import { authSession } from '$lib/auth/client';

  const cards = [
    {
      title: 'Auth Core',
      value: 'Live',
      note: 'JWT access token, refresh rotation, Redis session, dan one-device enforcement sudah aktif'
    },
    {
      title: '2FA Hardening',
      value: 'Ready',
      note: 'TOTP enrollment, recovery code, dan IP allowlist bisa dikelola dari halaman security'
    },
    {
      title: 'Next Domain',
      value: 'Stores',
      note: 'langkah berikutnya tinggal masuk store token, callback URL, dan staff scope'
    }
  ];
</script>

<svelte:head>
  <title>App | onixggr</title>
</svelte:head>

<section class="space-y-6">
  <div class="glass-panel rounded-[2rem] px-6 py-7">
    <p class="text-xs font-semibold uppercase tracking-[0.24em] text-accent-700">
      Dashboard Secure Area
    </p>
    <h2 class="mt-3 font-display text-4xl font-bold tracking-tight text-ink-900">
      Command center auth dan security
    </h2>
    <p class="mt-3 max-w-3xl text-sm leading-7 text-ink-700">
      Halaman app sekarang sudah menjadi area aman berbasis session dashboard. Gunakan halaman
      Security untuk menyalakan TOTP dan mengunci login ke satu IP bila diperlukan.
    </p>

    {#if $authSession}
      <div class="mt-6 rounded-[1.5rem] bg-canvas-100 px-5 py-4 text-sm text-ink-700">
        <p class="font-semibold text-ink-900">Current Session</p>
        <p class="mt-1">{$authSession.user.email}</p>
        <p>Role: {$authSession.user.role}</p>
      </div>
    {/if}
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
