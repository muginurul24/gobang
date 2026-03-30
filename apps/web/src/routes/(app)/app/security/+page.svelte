<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import QRCode from 'qrcode';

  import Button from '$lib/components/ui/button/button.svelte';
  import {
    authSession,
    beginTOTPEnrollment,
    disableTOTP,
    enableTOTP,
    fetchSecuritySettings,
    hydrateAuthSession,
    type SecuritySettings,
    type TOTPEnrollment,
    updateIPAllowlist
  } from '$lib/auth/client';

  let loading = true;
  let saving = false;
  let errorMessage = '';
  let successMessage = '';
  let security: SecuritySettings | null = null;
  let enrollment: TOTPEnrollment | null = null;
  let qrDataURL = '';
  let enableCode = '';
  let disableTOTPCode = '';
  let disableRecoveryCode = '';
  let ipAllowlist = '';
  let recoveryCodes: string[] = [];

  onMount(async () => {
    hydrateAuthSession();

    if (!$authSession) {
      await goto('/login');
      return;
    }

    await loadSecurity();
  });

  async function loadSecurity() {
    loading = true;
    errorMessage = '';

    const response = await fetchSecuritySettings();
    if (!response.status || response.message !== 'SUCCESS') {
      if (response.message === 'UNAUTHORIZED') {
        await goto('/login');
        return;
      }

      errorMessage = toMessage(response.message);
      loading = false;
      return;
    }

    security = response.data;
    ipAllowlist = response.data.ip_allowlist ?? '';
    loading = false;
  }

  async function startEnrollment() {
    errorMessage = '';
    successMessage = '';
    recoveryCodes = [];

    const response = await beginTOTPEnrollment();
    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    enrollment = response.data;
    qrDataURL = await QRCode.toDataURL(response.data.otpauth_url, {
      width: 240,
      margin: 1,
      color: {
        dark: '#10221a',
        light: '#f8fbf8'
      }
    });
  }

  async function activateTOTP() {
    if (!enrollment) {
      return;
    }

    saving = true;
    errorMessage = '';
    successMessage = '';

    const response = await enableTOTP(enableCode);
    saving = false;

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    recoveryCodes = response.data.codes;
    enrollment = null;
    qrDataURL = '';
    enableCode = '';
    successMessage = 'TOTP aktif. Recovery code hanya tampil sekali di bawah ini.';
    await loadSecurity();
  }

  async function deactivateTOTP() {
    saving = true;
    errorMessage = '';
    successMessage = '';

    const response = await disableTOTP({
      totp_code: disableTOTPCode || undefined,
      recovery_code: disableRecoveryCode || undefined
    });
    saving = false;

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    disableTOTPCode = '';
    disableRecoveryCode = '';
    recoveryCodes = [];
    successMessage = 'TOTP dinonaktifkan untuk akun ini.';
    await loadSecurity();
  }

  async function saveAllowlist() {
    saving = true;
    errorMessage = '';
    successMessage = '';

    const trimmed = ipAllowlist.trim();
    const response = await updateIPAllowlist(trimmed === '' ? null : trimmed);
    saving = false;

    if (!response.status || response.message !== 'SUCCESS') {
      errorMessage = toMessage(response.message);
      return;
    }

    security = response.data;
    ipAllowlist = response.data.ip_allowlist ?? '';
    successMessage = trimmed === '' ? 'IP allowlist dibersihkan.' : 'IP allowlist tersimpan.';
  }

  function toMessage(message: string) {
    switch (message) {
      case 'UNAUTHORIZED':
        return 'Sesi tidak lagi valid. Silakan login ulang.';
      case 'INVALID_2FA_CODE':
        return 'Kode TOTP atau recovery code tidak valid.';
      case 'TOTP_ALREADY_ENABLED':
        return 'TOTP sudah aktif untuk akun ini.';
      case 'TOTP_NOT_ENABLED':
        return 'TOTP belum aktif.';
      case 'NO_PENDING_ENROLLMENT':
        return 'Mulai enrollment TOTP baru terlebih dahulu.';
      case 'INVALID_IP_ALLOWLIST':
        return 'Format IP allowlist tidak valid.';
      default:
        return 'Terjadi kesalahan. Coba ulangi.';
    }
  }
</script>

<svelte:head>
  <title>Security | onixggr</title>
</svelte:head>

{#if loading}
  <div class="glass-panel rounded-4xl p-6">
    <p class="text-sm text-ink-700">Memuat konfigurasi keamanan akun...</p>
  </div>
{:else if security}
  <div class="space-y-6">
    <section class="glass-panel rounded-4xl p-6">
      <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div class="space-y-2">
          <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">
            Security Center
          </p>
          <h1 class="font-display text-3xl font-bold tracking-tight text-ink-900">
            TOTP, recovery code, dan IP allowlist
          </h1>
          <p class="max-w-2xl text-sm leading-6 text-ink-700">
            Blueprint meminta 2FA TOTP opsional tapi strongly recommended, recovery code sekali
            pakai, dan IP allowlist single IP untuk login dashboard.
          </p>
        </div>

        <div class="rounded-3xl bg-canvas-100 px-4 py-3 text-sm text-ink-700">
          <p class="font-semibold text-ink-900">Status</p>
          <p>{security.totp_enabled ? '2FA aktif' : '2FA belum aktif'}</p>
          <p>{security.ip_allowlist ? `Allowlist: ${security.ip_allowlist}` : 'Allowlist kosong'}</p>
        </div>
      </div>
    </section>

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

    {#if !security.totp_enabled}
      <section class="glass-panel rounded-4xl p-6">
        <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
          <div class="space-y-2">
            <h2 class="font-display text-2xl font-bold text-ink-900">Aktifkan TOTP</h2>
            <p class="max-w-2xl text-sm leading-6 text-ink-700">
              Aktifkan 2FA sebelum akun owner dipakai operasional. Flow ini akan menghasilkan
              recovery code sekali pakai yang wajib Anda simpan.
            </p>
          </div>

          <Button variant="brand" size="lg" onclick={startEnrollment} disabled={saving}>
            Mulai Enrollment
          </Button>
        </div>

        {#if enrollment}
          <div class="mt-6 grid gap-6 lg:grid-cols-[0.9fr_1.1fr]">
            <div class="rounded-3xl border border-ink-100 bg-canvas-50 p-5">
              {#if qrDataURL}
                <img alt="QR code TOTP" class="mx-auto rounded-3xl bg-white p-3" src={qrDataURL} />
              {/if}
            </div>

            <div class="space-y-4">
              <div class="rounded-3xl border border-ink-100 bg-white px-4 py-4">
                <p class="text-xs font-semibold uppercase tracking-[0.2em] text-ink-300">
                  Secret Manual
                </p>
                <p class="mt-2 font-mono text-sm text-ink-900">{enrollment.secret}</p>
              </div>

              <div class="space-y-2">
                <label class="block space-y-2">
                  <span class="text-sm font-medium text-ink-700">Kode 6 digit dari authenticator</span>
                  <input
                    bind:value={enableCode}
                    class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                    inputmode="numeric"
                    maxlength="6"
                    placeholder="654321"
                  />
                </label>
                <p class="text-xs leading-5 text-ink-300">
                  Enrollment ini berlaku sampai {new Date(enrollment.expires_at).toLocaleString()}.
                </p>
              </div>

              <Button variant="brand" size="lg" class="w-full" onclick={activateTOTP} disabled={saving}>
                Aktivasi TOTP
              </Button>
            </div>
          </div>
        {/if}

        {#if recoveryCodes.length > 0}
          <div class="mt-6 rounded-3xl border border-amber-200 bg-amber-50 px-5 py-5">
            <p class="text-sm font-semibold text-amber-900">Recovery code sekali pakai</p>
            <p class="mt-2 text-sm leading-6 text-amber-800">
              Simpan daftar ini sekarang. Setelah halaman ditutup, plain recovery code tidak akan
              bisa ditampilkan lagi.
            </p>
            <div class="mt-4 grid gap-3 sm:grid-cols-2">
              {#each recoveryCodes as code}
                <div class="rounded-2xl border border-amber-200 bg-white px-4 py-3 font-mono text-sm text-ink-900">
                  {code}
                </div>
              {/each}
            </div>
          </div>
        {/if}
      </section>
    {:else}
      <section class="glass-panel rounded-4xl p-6">
        <div class="space-y-2">
          <h2 class="font-display text-2xl font-bold text-ink-900">Nonaktifkan TOTP</h2>
          <p class="max-w-2xl text-sm leading-6 text-ink-700">
            Untuk menonaktifkan 2FA, masukkan satu kode TOTP aktif atau satu recovery code yang
            belum dipakai.
          </p>
        </div>

        <div class="mt-6 grid gap-4 md:grid-cols-2">
          <label class="block space-y-2">
            <span class="text-sm font-medium text-ink-700">Kode TOTP</span>
            <input
              bind:value={disableTOTPCode}
              class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
              inputmode="numeric"
              maxlength="6"
              placeholder="654321"
            />
          </label>

          <label class="block space-y-2">
            <span class="text-sm font-medium text-ink-700">Atau recovery code</span>
            <input
              bind:value={disableRecoveryCode}
              class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 font-mono text-sm text-ink-900 outline-none transition focus:border-accent-300"
              placeholder="ABCD3-EFGH4"
            />
          </label>
        </div>

        <div class="mt-5 flex justify-end">
          <Button variant="outline" size="lg" onclick={deactivateTOTP} disabled={saving}>
            Nonaktifkan TOTP
          </Button>
        </div>
      </section>
    {/if}

    <section class="glass-panel rounded-4xl p-6">
      <div class="space-y-2">
        <h2 class="font-display text-2xl font-bold text-ink-900">IP Allowlist</h2>
        <p class="max-w-2xl text-sm leading-6 text-ink-700">
          Berlaku hanya untuk login dashboard. Satu user hanya boleh memiliki satu IP allowlist.
          Kosongkan field ini bila Anda ingin menonaktifkannya.
        </p>
      </div>

      <div class="mt-6 grid gap-4 md:grid-cols-[1fr_auto]">
        <label class="block space-y-2">
          <span class="text-sm font-medium text-ink-700">Single IPv4 / IPv6</span>
          <input
            bind:value={ipAllowlist}
            class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
            placeholder="203.0.113.10"
          />
        </label>

        <div class="flex items-end">
          <Button variant="brand" size="lg" onclick={saveAllowlist} disabled={saving}>
            Simpan Allowlist
          </Button>
        </div>
      </div>
    </section>
  </div>
{/if}
