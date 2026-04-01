<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import Button from '$lib/components/ui/button/button.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import { authSession, initializeAuthSession, login, saveAuthSession } from '$lib/auth/client';

  let loginValue = '';
  let password = '';
  let totpCode = '';
  let recoveryCode = '';
  let loading = false;
  let errorMessage = '';
  let successMessage = '';
  let requiresTOTP = false;

  onMount(async () => {
    await initializeAuthSession();

    if ($authSession) {
      await goto('/app');
    }
  });

  async function submitLogin() {
    loading = true;
    errorMessage = '';
    successMessage = '';

    const response = await login({
      login: loginValue,
      password,
      totp_code: totpCode || undefined,
      recovery_code: recoveryCode || undefined
    });

    loading = false;

    if (response.status && response.message === 'SUCCESS') {
      saveAuthSession(response.data);
      successMessage = 'Login berhasil. Mengalihkan ke dashboard...';
      await goto('/app');
      return;
    }

    if (response.message === 'TOTP_REQUIRED') {
      requiresTOTP = true;
      errorMessage = '';
      return;
    }

    errorMessage = toMessage(response.message);
  }

  function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    void submitLogin();
  }

  function toMessage(message: string) {
    switch (message) {
      case 'INVALID_CREDENTIALS':
        return 'Email/username atau password tidak valid.';
      case 'INVALID_2FA_CODE':
        return 'Kode TOTP atau recovery code tidak valid.';
      case 'IP_NOT_ALLOWED':
        return 'IP ini tidak ada di allowlist user.';
      case 'RATE_LIMITED':
        return 'Terlalu banyak percobaan login. Tunggu beberapa saat lalu coba lagi.';
      default:
        return 'Terjadi kesalahan saat login.';
    }
  }
</script>

<svelte:head>
  <title>Login | onixggr</title>
</svelte:head>

<main class="space-y-6" id="app-main">
  <div class="space-y-3">
    <p class="section-kicker">Command Login</p>
    <h1 class="font-display text-4xl font-bold tracking-tight text-ink-900 sm:text-5xl">
      Masuk ke command center
    </h1>
    <p class="max-w-2xl text-sm leading-7 text-ink-700">
      Gunakan username atau email dashboard. Jika akun mewajibkan 2FA, flow akan otomatis
      berpindah ke verifikasi TOTP atau recovery code.
    </p>
  </div>

  <div class="grid gap-3 sm:grid-cols-3">
    {#each [
      ['Session', 'refresh cookie + CSRF'],
      ['Boundary', 'owner/store scope'],
      ['Hardening', '2FA + allowlist']
    ] as [label, value]}
      <article class="rounded-[1.6rem] border border-white/60 bg-white/70 px-4 py-4 shadow-[0_16px_32px_rgba(7,16,12,0.08)]">
        <p class="text-[0.68rem] font-semibold uppercase tracking-[0.24em] text-ink-300">{label}</p>
        <p class="mt-2 text-sm font-semibold text-ink-900">{value}</p>
      </article>
    {/each}
  </div>

  {#if errorMessage}
    <Notice tone="error" title="Login Gagal" message={errorMessage} />
  {/if}

  {#if successMessage}
    <Notice tone="success" title="Akses Diterima" message={successMessage} />
  {/if}

  <form class="space-y-4" onsubmit={handleSubmit}>
    <label class="block space-y-2">
      <span class="text-sm font-medium text-ink-700">Email atau username</span>
      <input
        bind:value={loginValue}
        class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
        type="text"
        placeholder="owner@example.com"
      />
    </label>

    <label class="block space-y-2">
      <span class="text-sm font-medium text-ink-700">Password</span>
      <input
        bind:value={password}
        class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
        type="password"
        placeholder="••••••••"
      />
    </label>

    {#if requiresTOTP}
      <div class="rounded-[1.7rem] border border-accent-200/60 bg-linear-to-r from-accent-100/70 to-white px-4 py-4">
        <p class="text-sm font-semibold text-ink-900">2FA diperlukan</p>
        <p class="mt-1 text-sm leading-6 text-ink-700">
          Isi salah satu: kode TOTP 6 digit dari authenticator atau recovery code sekali pakai.
        </p>
      </div>

      <label class="block space-y-2">
        <span class="text-sm font-medium text-ink-700">Kode TOTP</span>
        <input
          bind:value={totpCode}
          class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
          inputmode="numeric"
          maxlength="6"
          placeholder="654321"
        />
      </label>

      <label class="block space-y-2">
        <span class="text-sm font-medium text-ink-700">Atau recovery code</span>
        <input
          bind:value={recoveryCode}
          class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 font-mono text-sm text-ink-900 outline-none transition focus:border-accent-300"
          placeholder="ABCD3-EFGH4"
        />
      </label>
    {/if}

    <Button variant="brand" size="lg" class="w-full" type="submit" disabled={loading}>
      {requiresTOTP ? 'Verifikasi Login' : 'Masuk ke Dashboard'}
    </Button>
  </form>

  <article class="rounded-[1.7rem] border border-dashed border-ink-200 bg-white/56 px-4 py-4 text-sm leading-6 text-ink-700">
    <p class="font-semibold text-ink-900">Catatan environment</p>
    <p class="mt-2">
      Seed demo atau akun bootstrap bisa berbeda per environment. UI login tidak lagi mengisi demo
      credential secara otomatis agar aman untuk staging dan production.
    </p>
  </article>
</main>
