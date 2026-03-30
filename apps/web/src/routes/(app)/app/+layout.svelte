<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import Button from '$lib/components/ui/button/button.svelte';
  import {
    authSession,
    hydrateAuthSession,
    logoutCurrentSession,
    syncProfile
  } from '$lib/auth/client';

  let ready = false;

  $: role = $authSession?.user.role ?? '';
  $: nav = [
    { href: '/app', label: 'Dashboard' },
    { href: '/app/stores', label: 'Stores' },
    ...(role === 'karyawan' ? [] : [{ href: '/app/audit', label: 'Audit' }]),
    { href: '/app/security', label: 'Security' },
    { href: '/app/chat', label: 'Global Chat' },
    { href: '/', label: 'Back to Public' }
  ];

  onMount(async () => {
    hydrateAuthSession();

    if (!$authSession) {
      await goto('/login');
      return;
    }

    const profile = await syncProfile();
    if (!profile.status || profile.message !== 'SUCCESS') {
      await goto('/login');
      return;
    }

    ready = true;
  });

  async function signOut() {
    await logoutCurrentSession();
    await goto('/login');
  }
</script>

{#if ready}
  <div class="shell-width mx-auto flex min-h-screen flex-col gap-6 py-6 lg:flex-row">
    <aside class="glass-panel w-full rounded-[2rem] p-5 lg:sticky lg:top-6 lg:h-[calc(100vh-3rem)] lg:w-80">
      <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">App Shell</p>
      <h1 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">onixggr</h1>
      <p class="mt-3 text-sm leading-6 text-ink-700">
        Area app sekarang menutup auth, store management, audit viewer, dan security hardening dari
        blueprint awal.
      </p>

      {#if $authSession}
        <div class="mt-6 rounded-[1.5rem] bg-canvas-100 px-4 py-4 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Signed In</p>
          <p class="mt-1">{$authSession.user.username}</p>
          <p>{$authSession.user.role}</p>
        </div>
      {/if}

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

      <div class="mt-8">
        <Button variant="outline" size="lg" class="w-full" onclick={signOut}>
          Logout
        </Button>
      </div>
    </aside>

    <main class="min-w-0 flex-1">
      <slot />
    </main>
  </div>
{:else}
  <div class="shell-width mx-auto min-h-screen py-10">
    <div class="glass-panel rounded-[2rem] px-6 py-8">
      <p class="text-sm text-ink-700">Memeriksa session dashboard...</p>
    </div>
  </div>
{/if}
