<script lang="ts">
  import { onMount } from 'svelte';

  import EmptyState from '$lib/components/app/empty-state.svelte';
  import MetricCard from '$lib/components/app/metric-card.svelte';
  import Notice from '$lib/components/app/notice.svelte';
  import StoreScopePicker from '$lib/components/app/store-scope-picker.svelte';
  import Button from '$lib/components/ui/button/button.svelte';
  import { authSession } from '$lib/auth/client';
  import { formatNumber } from '$lib/formatters';
  import type { Store } from '$lib/stores/client';
  import { hydratePreferredStoreID, preferredStoreID, setPreferredStoreID } from '$lib/stores/preferences';

  type EndpointDoc = {
    method: 'GET' | 'POST';
    path: string;
    title: string;
    body: string;
    request?: string;
  };

  const endpointDocs: EndpointDoc[] = [
    {
      method: 'POST',
      path: '/v1/store-api/game/users',
      title: 'Create member mapping',
      body: 'Buat user upstream untuk satu real username milik toko.',
      request: `{\n  "username": "member-alpha"\n}`,
    },
    {
      method: 'GET',
      path: '/v1/store-api/game/balance?username=member-alpha',
      title: 'Read member balance',
      body: 'Ambil saldo terbaru member dari upstream game provider.',
    },
    {
      method: 'POST',
      path: '/v1/store-api/game/deposits',
      title: 'Game deposit',
      body: 'Debit saldo toko saat deposit member berhasil.',
      request: `{\n  "username": "member-alpha",\n  "amount": 5000,\n  "trx_id": "trx-deposit-001"\n}`,
    },
    {
      method: 'POST',
      path: '/v1/store-api/game/withdrawals',
      title: 'Game withdraw',
      body: 'Credit saldo toko saat withdraw member berhasil.',
      request: `{\n  "username": "member-alpha",\n  "amount": 5000,\n  "trx_id": "trx-withdraw-001"\n}`,
    },
    {
      method: 'POST',
      path: '/v1/store-api/game/launch',
      title: 'Launch game',
      body: 'Launch session game dengan provider_code dan game_code yang aktif.',
      request: `{\n  "username": "member-alpha",\n  "provider_code": "PRAGMATIC",\n  "game_code": "vs20doghouse",\n  "lang": "id"\n}`,
    },
    {
      method: 'POST',
      path: '/v1/store-api/qris/member-payments',
      title: 'Generate member QRIS',
      body: 'Generate QRIS member payment untuk deposit manual melalui store token.',
      request: `{\n  "username": "member-alpha",\n  "amount": 25000\n}`,
    },
  ];

  let storeScopeLoading = true;
  let storeScopeTotalCount = 0;
  let selectedStoreID = '';
  let selectedStore: Store | null = null;
  let baseURL = 'https://app.bola788.store';
  let copiedKey = '';
  let endpointSearch = '';
  let callbackTesterSecret = '';
  let callbackTesterPayload = '';
  let callbackTesterSignature = '';
  let callbackTesterBusy = false;
  let callbackTesterError = '';

  $: currentStore = selectedStore;
  $: role = $authSession?.user.role ?? '';
  $: callbackHint =
    role === 'owner' || role === 'superadmin'
      ? 'Atur callback URL toko dari halaman Stores, lalu sinkronkan endpoint website Anda.'
      : 'Callback URL dikelola oleh owner atau superadmin dari halaman Stores.';
  $: bootstrapSteps = [
    'Rotate dan simpan store token dari halaman Stores. Token hanya tampil sekali saat create atau rotate.',
    'Daftarkan callback URL toko agar event member_payment.success bisa dikirim balik ke website Anda.',
    'Gunakan real username konsisten. upstream_user_code akan dikelola backend dan tidak perlu Anda buat sendiri.',
    'Pastikan setiap transaksi uang memakai trx_id unik supaya retry aman dan idempotent.',
  ];
  $: responseEnvelopeExample = `{\n  "status": true,\n  "message": "SUCCESS",\n  "data": {\n    "id": "uuid",\n    "custom_ref": "TOPUP-001",\n    "status": "pending"\n  }\n}`;
  $: callbackExample = `POST ${currentStore?.callback_url || 'https://merchant.example.com/callback'}\nHeaders:\n  Content-Type: application/json\n  X-Onixggr-Signature: sha256=<hmac>\n\nBody contoh event outbound ke website owner:\n{\n  "event_type": "member_payment.success",\n  "occurred_at": "${new Date().toISOString()}",\n  "reference_type": "qris_transaction",\n  "reference_id": "trx_123",\n  "data": {\n    "qris_transaction_id": "trx_123",\n    "store_id": "${currentStore?.id ?? 'store_uuid'}",\n    "store_member_id": "member_uuid",\n    "real_username": "member-alpha",\n    "status": "success",\n    "custom_ref": "ORDER-2026-001",\n    "provider_trx_id": "provider-rrn-001",\n    "amount_gross": "25000.00",\n    "platform_fee_amount": "750.00",\n    "store_credit_amount": "24250.00",\n    "paid_at": "${new Date().toISOString()}"\n  }\n}`;
  $: signatureCheckNode = `import crypto from 'node:crypto';\n\nfunction verifyOnixSignature(rawBody, signatureHeader, secret) {\n  const expected = 'sha256=' + crypto.createHmac('sha256', secret).update(rawBody).digest('hex');\n  return crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(signatureHeader || ''));\n}`;
  $: errorDeck = [
    {
      code: 'UNAUTHORIZED',
      meaning: 'Store token salah, expired, atau dashboard browser session tidak valid.',
    },
    {
      code: 'PENDING_PROVIDER',
      meaning: 'Response QRIS ambigu atau timeout. Jangan anggap sukses, tunggu webhook/reconcile.',
    },
    {
      code: 'INVALID_PROVIDER_GAME',
      meaning: 'provider_code atau game_code tidak aktif di katalog lokal hasil sync.',
    },
    {
      code: 'IDEMPOTENCY_KEY_CONFLICT',
      meaning: 'Withdraw key dipakai ulang untuk intent yang berbeda.',
    },
  ];

  $: visibleEndpointDocs = endpointDocs.filter((endpoint) => {
    const query = endpointSearch.trim().toLowerCase();
    if (query === '') {
      return true;
    }

    return [endpoint.method, endpoint.path, endpoint.title, endpoint.body, endpoint.request ?? '']
      .join(' ')
      .toLowerCase()
      .includes(query);
  });
  $: selectedExamples = visibleEndpointDocs.map((endpoint) => ({
    ...endpoint,
    curl: buildCurl(endpoint),
  }));

  onMount(() => {
    hydratePreferredStoreID();
    selectedStoreID = getPreferredStoreID();
    baseURL =
      (import.meta.env.PUBLIC_API_BASE_URL ?? '').trim() || window.location.origin;
    callbackTesterPayload = sampleCallbackPayload();
  });

  async function copySnippet(key: string, value: string) {
    try {
      await navigator.clipboard.writeText(value);
      copiedKey = key;
      window.setTimeout(() => {
        if (copiedKey === key) {
          copiedKey = '';
        }
      }, 1800);
    } catch {
      copiedKey = '';
    }
  }

  function handleStoreScopeChange(event: CustomEvent<{ storeID: string; store: Store | null }>) {
    selectedStoreID = event.detail.storeID;
    selectedStore = event.detail.store;
    setPreferredStoreID(selectedStoreID);
  }

  function buildCurl(endpoint: EndpointDoc) {
    const lines = [
      `curl -X ${endpoint.method} \\`,
      `  '${baseURL}${endpoint.path}' \\`,
      `  -H 'Authorization: Bearer STORE_TOKEN_HERE' \\`,
      `  -H 'Content-Type: application/json'`,
    ];

    if (endpoint.request) {
      lines.push(`  -d '${endpoint.request.replace(/\n/g, '\n  ')}'`);
    }

    return lines.join('\n');
  }

  function getPreferredStoreID() {
    let current = '';
    const unsubscribe = preferredStoreID.subscribe((value) => {
      current = value;
    });
    unsubscribe();
    return current;
  }

  function pillClass(method: 'GET' | 'POST') {
    return method === 'GET'
      ? 'bg-brand-100 text-brand-700'
      : 'bg-accent-100 text-accent-700';
  }

  async function generateCallbackSignature() {
    callbackTesterError = '';
    callbackTesterSignature = '';

    const secret = callbackTesterSecret.trim();
    const payload = callbackTesterPayload.trim();
    if (secret === '' || payload === '') {
      callbackTesterError = 'Secret dan payload wajib diisi.';
      return;
    }

    if (!globalThis.crypto?.subtle) {
      callbackTesterError = 'Web Crypto tidak tersedia di browser ini.';
      return;
    }

    callbackTesterBusy = true;

    try {
      const encoder = new TextEncoder();
      const key = await globalThis.crypto.subtle.importKey(
        'raw',
        encoder.encode(secret),
        { name: 'HMAC', hash: 'SHA-256' },
        false,
        ['sign']
      );
      const signature = await globalThis.crypto.subtle.sign(
        'HMAC',
        key,
        encoder.encode(payload)
      );
      callbackTesterSignature = `sha256=${toHex(signature)}`;
    } catch (error) {
      callbackTesterError =
        error instanceof Error
          ? error.message
          : 'Gagal menghasilkan signature callback.';
    } finally {
      callbackTesterBusy = false;
    }
  }

  function loadSamplePayload() {
    callbackTesterPayload = sampleCallbackPayload();
    callbackTesterError = '';
    callbackTesterSignature = '';
  }

  function sampleCallbackPayload() {
    return JSON.stringify(
      {
        event_type: 'member_payment.success',
        occurred_at: new Date().toISOString(),
        reference_type: 'qris_transaction',
        reference_id: 'trx_123',
        data: {
          qris_transaction_id: 'trx_123',
          store_id: currentStore?.id ?? 'store_uuid',
          store_member_id: 'member_uuid',
          real_username: 'member-alpha',
          status: 'success',
          custom_ref: 'ORDER-2026-001',
          provider_trx_id: 'provider-rrn-001',
          amount_gross: '25000.00',
          platform_fee_amount: '750.00',
          store_credit_amount: '24250.00',
          paid_at: new Date().toISOString(),
        },
      },
      null,
      2,
    );
  }

  function toHex(buffer: ArrayBuffer) {
    return Array.from(new Uint8Array(buffer))
      .map((value) => value.toString(16).padStart(2, '0'))
      .join('');
  }
</script>

<svelte:head>
  <title>API Docs | onixggr</title>
</svelte:head>

<section class="space-y-6">
  <section class="surface-dark surface-grid overflow-hidden rounded-[2.4rem] px-6 py-6 text-white sm:px-7 sm:py-7">
    <div class="grid gap-6 xl:grid-cols-[1.08fr_0.92fr]">
      <div class="space-y-4">
        <span class="status-chip w-fit">Owner integration docs</span>
        <div class="space-y-3">
          <p class="section-kicker">Store API playbook</p>
          <h1 class="font-display text-4xl font-bold tracking-tight sm:text-5xl">
            Dokumentasi integrasi praktis untuk website owner.
          </h1>
          <p class="max-w-3xl text-sm leading-7 text-white/72 sm:text-base">
            Halaman ini merangkum endpoint store API, pola callback, dan langkah bootstrap integrasi
            yang sesuai dengan kontrak backend yang sudah ada di project ini.
          </p>
        </div>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <MetricCard
          class="h-full"
          eyebrow="Base URL"
          title="Public origin"
          value={baseURL}
          detail="Semua contoh request di bawah menggunakan origin publik yang sedang dibuka user."
          tone="brand"
        />
        <MetricCard
          class="h-full"
          eyebrow="Selected Store"
          title={currentStore?.name ?? 'No active store'}
          value={currentStore?.slug ?? '-'}
          detail={callbackHint}
          tone="accent"
        />
      </div>
    </div>
  </section>

  <div class="grid gap-6 xl:grid-cols-[0.82fr_1.18fr]">
      <section class="space-y-6">
        <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Bootstrap</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Langkah integrasi
              </h2>
            </div>
            <span class="surface-chip">{formatNumber(storeScopeTotalCount)} store</span>
          </div>

          <div class="mt-5">
            <StoreScopePicker
              bind:selectedStoreID
              bind:selectedStore
              bind:loading={storeScopeLoading}
              bind:totalCount={storeScopeTotalCount}
              compact
              title="Store scope untuk dokumentasi"
              description="Selector docs memakai store directory backend yang sama dengan halaman operasional, jadi tetap cepat saat tenant roster membesar."
              placeholder="Cari store untuk contoh token dan callback"
              on:change={handleStoreScopeChange}
            />
          </div>

          <ol class="mt-5 space-y-3">
            {#each bootstrapSteps as step, index}
              <li class="rounded-[1.5rem] border border-ink-100 bg-canvas-50 px-4 py-4 text-sm leading-6 text-ink-700">
                <span class="mb-2 inline-flex h-8 w-8 items-center justify-center rounded-full bg-canvas-900 font-mono text-xs font-semibold text-white">
                  {index + 1}
                </span>
                <p>{step}</p>
              </li>
            {/each}
          </ol>
        </article>

        <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
          <p class="section-kicker !text-brand-700">Callback contract</p>
          <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
            Outbound callback reminder
          </h2>
          <p class="mt-3 text-sm leading-7 text-ink-700">
            Simpan `CALLBACK_SIGNING_SECRET` yang sama di website owner Anda lalu verifikasi HMAC
            header sebelum menerima event final seperti `member_payment.success`.
          </p>

          <pre class="code-block mt-5">{callbackExample}</pre>
        </article>

        <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Callback playground</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Generate HMAC header locally
              </h2>
            </div>
            <span class="surface-chip">local only</span>
          </div>

          <p class="mt-3 text-sm leading-7 text-ink-700">
            Secret tidak dikirim ke backend. Browser hanya memakai Web Crypto untuk menghasilkan
            header `X-Onixggr-Signature` dari payload callback yang Anda siapkan sendiri.
          </p>

          <div class="mt-5 grid gap-4">
            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">CALLBACK_SIGNING_SECRET</span>
              <input
                bind:value={callbackTesterSecret}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                placeholder="Masukkan secret callback owner"
                type="password"
              />
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Payload callback</span>
              <textarea
                bind:value={callbackTesterPayload}
                class="min-h-[240px] w-full rounded-[1.5rem] border border-ink-100 bg-white px-4 py-4 font-mono text-xs leading-6 text-ink-900 outline-none transition focus:border-accent-300"
                spellcheck="false"
              ></textarea>
            </label>

            <div class="flex flex-wrap gap-2">
              <Button variant="brand" size="sm" onclick={generateCallbackSignature}>
                {callbackTesterBusy ? 'Generating…' : 'Generate signature'}
              </Button>
              <Button variant="outline" size="sm" onclick={loadSamplePayload}>
                Load sample payload
              </Button>
              <Button
                variant="outline"
                size="sm"
                onclick={() => copySnippet('callback-payload', callbackTesterPayload)}
              >
                {copiedKey === 'callback-payload' ? 'Copied payload' : 'Copy payload'}
              </Button>
            </div>

            {#if callbackTesterError !== ''}
              <Notice tone="warning" message={callbackTesterError} />
            {/if}

            <div class="rounded-[1.6rem] border border-ink-100 bg-canvas-50 px-4 py-4">
              <p class="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-ink-300">
                X-Onixggr-Signature
              </p>
              <p class="mt-3 break-all font-mono text-xs leading-6 text-ink-900">
                {callbackTesterSignature || 'sha256=<generated-hmac-signature>'}
              </p>
              <div class="mt-4 flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={callbackTesterSignature === ''}
                  onclick={() => copySnippet('callback-signature', callbackTesterSignature)}
                >
                  {copiedKey === 'callback-signature' ? 'Copied signature' : 'Copy signature'}
                </Button>
              </div>
            </div>
          </div>
        </article>

        <div class="grid gap-6 xl:grid-cols-2">
          <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
            <p class="section-kicker !text-brand-700">Envelope contract</p>
            <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
              Response shape
            </h2>
            <p class="mt-3 text-sm leading-7 text-ink-700">
              Browser dashboard dan website owner sama-sama menerima envelope `status`, `message`,
              dan `data`. Gunakan `message` untuk branch behaviour, bukan sekadar HTTP code.
            </p>

            <pre class="code-block mt-5">{responseEnvelopeExample}</pre>
          </article>

          <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
            <p class="section-kicker !text-brand-700">Signature verify</p>
            <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
              Node callback verifier
            </h2>
            <p class="mt-3 text-sm leading-7 text-ink-700">
              Pakai raw request body, bukan JSON yang sudah diparse ulang, agar HMAC Anda identik
              dengan payload yang dikirim onixggr.
            </p>

            <pre class="code-block mt-5">{signatureCheckNode}</pre>
          </article>
        </div>
      </section>

      <section class="space-y-6">
        <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <p class="section-kicker !text-brand-700">Endpoint deck</p>
              <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
                Store API endpoints
              </h2>
            </div>
            <a class="surface-chip" href="/app/stores">Open Stores</a>
          </div>

          <div class="mt-5 grid gap-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
            <label class="space-y-2">
              <span class="text-sm font-medium text-ink-700">Search endpoint examples</span>
              <input
                bind:value={endpointSearch}
                class="w-full rounded-2xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 outline-none transition focus:border-accent-300"
                placeholder="Cari qris, withdrawals, launch, callback, atau balance"
                type="search"
              />
            </label>

            <span class="surface-chip">{selectedExamples.length} endpoint</span>
          </div>

          <div class="mt-6 space-y-4">
            {#if selectedExamples.length === 0}
              <EmptyState
                eyebrow="Endpoint Search"
                title="Tidak ada endpoint yang cocok"
                body="Ubah keyword pencarian untuk melihat contoh cURL dan flow integrasi owner yang relevan."
              />
            {:else}
              {#each selectedExamples as endpoint}
                <article class="rounded-[1.7rem] border border-ink-100 bg-white/78 p-4 shadow-[0_16px_34px_rgba(7,16,12,0.08)]">
                  <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div>
                      <div class="flex flex-wrap items-center gap-2">
                        <span class={`rounded-full px-3 py-1 text-[0.68rem] font-semibold uppercase tracking-[0.22em] ${pillClass(endpoint.method)}`}>
                          {endpoint.method}
                        </span>
                        <p class="font-mono text-sm text-ink-900">{endpoint.path}</p>
                      </div>
                      <h3 class="mt-3 text-lg font-semibold text-ink-900">{endpoint.title}</h3>
                      <p class="mt-2 text-sm leading-6 text-ink-700">{endpoint.body}</p>
                    </div>

                    <Button
                      variant="outline"
                      size="sm"
                      onclick={() => copySnippet(endpoint.path, endpoint.curl)}
                    >
                      {copiedKey === endpoint.path ? 'Copied' : 'Copy cURL'}
                    </Button>
                  </div>

                  <pre class="code-block mt-5">{endpoint.curl}</pre>
                </article>
              {/each}
            {/if}
          </div>
        </article>

        <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
          <p class="section-kicker !text-brand-700">Rules to remember</p>
          <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
            Business constraints
          </h2>
          <ul class="mt-5 space-y-3 text-sm leading-7 text-ink-700">
            <li class="rounded-[1.4rem] bg-canvas-50 px-4 py-4">
              1 store hanya punya 1 token aktif pada satu waktu. Rotate berarti token lama langsung mati.
            </li>
            <li class="rounded-[1.4rem] bg-canvas-50 px-4 py-4">
              `member_payment`, `store_topup`, `game deposit`, `game withdraw`, dan `withdraw dashboard`
              adalah domain transaksi yang berbeda. Jangan campur idempotency atau status flow-nya.
            </li>
            <li class="rounded-[1.4rem] bg-canvas-50 px-4 py-4">
              Timeout atau response ambigu tidak boleh diasumsikan sukses di sisi owner. Tunggu callback
              final atau hasil reconcile worker.
            </li>
          </ul>
        </article>

        <article class="glass-panel rounded-[2.2rem] p-5 sm:p-6">
          <p class="section-kicker !text-brand-700">Failure map</p>
          <h2 class="mt-3 font-display text-3xl font-bold tracking-tight text-ink-900">
            Error deck yang paling sering dipakai
          </h2>

          <div class="mt-5 space-y-3">
            {#each errorDeck as item}
              <article class="rounded-[1.5rem] border border-ink-100 bg-canvas-50 px-4 py-4">
                <div class="flex flex-wrap items-center gap-3">
                  <span class="surface-chip">{item.code}</span>
                </div>
                <p class="mt-3 text-sm leading-7 text-ink-700">{item.meaning}</p>
              </article>
            {/each}
          </div>
        </article>
      </section>
    </div>
</section>
