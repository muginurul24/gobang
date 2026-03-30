<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  import Button from '$lib/components/ui/button/button.svelte';
  import { authSession, hydrateAuthSession, login, saveAuthSession } from '$lib/auth/client';

  let loginValue = 'owner-demo';
  let password = 'OwnerDemo123!';
  let totpCode = '';
  let recoveryCode = '';
  let loading = false;
  let errorMessage = '';
  let successMessage = '';
  let requiresTOTP = false;

  onMount(async () => {
    hydrateAuthSession();

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

<div class="space-y-6">
  <div class="space-y-2">
    <p class="text-xs font-semibold uppercase tracking-[0.24em] text-accent-700">Auth UX</p>
    <h1 class="font-display text-3xl font-bold tracking-tight text-ink-900">Masuk ke dashboard</h1>
    <p class="text-sm leading-6 text-ink-700">
      Login dashboard sekarang sudah mendukung username/email, one-device session, TOTP optional,
      recovery code, dan hardening rate limit dasar.
    </p>
  </div>

  <div class="rounded-3xl border border-brand-200 bg-brand-100/60 px-4 py-4 text-sm leading-6 text-brand-700">
    Demo:
    <span class="font-semibold text-ink-900">owner-demo / OwnerDemo123!</span>
    atau
    <span class="font-semibold text-ink-900">dev-demo / DevDemo123!</span>
  </div>

  {#if errorMessage}
    <div class="rounded-3xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
      {errorMessage}
    </div>
  {/if}

  {#if successMessage}
    <div class="rounded-3xl border border-brand-200 bg-brand-100/60 px-4 py-3 text-sm text-brand-700">
      {successMessage}
    </div>
  {/if}

  <form class="space-y-4" onsubmit={handleSubmit}>
    <label class="block space-y-2">
      <span class="text-sm font-medium text-ink-700">Email atau username</span>
      <input
        bind:value={loginValue}
        class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
        type="text"
        placeholder="owner-demo"
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
      <div class="rounded-3xl border border-ink-100 bg-canvas-50 px-4 py-4">
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
</div>
