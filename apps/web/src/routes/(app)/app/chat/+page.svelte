<svelte:head>
  <title>Chat | onixggr</title>
</svelte:head>

<script lang="ts">
  import Button from '$lib/components/ui/button/button.svelte';
  import { realtimeState, sendRealtimePing } from '$lib/realtime/client';

  function pingRealtime() {
    sendRealtimePing();
  }
</script>

<section class="glass-panel overflow-hidden rounded-4xl">
  <header class="border-b border-ink-100 px-6 py-5">
    <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">Global Chat</p>
    <h2 class="mt-2 font-display text-3xl font-bold tracking-tight text-ink-900">
      Backbone realtime sekarang aktif untuk channel user, store, role, dan global chat.
    </h2>
  </header>

  <div class="grid gap-4 p-6 lg:grid-cols-[1fr_18rem]">
    <div class="space-y-3">
      <article class="rounded-3xl bg-canvas-100 px-5 py-4 text-sm leading-6 text-ink-700">
        Hari ini fokusnya masih transport: websocket auth, channel binding, heartbeat, dan
        reconnect. Stream message chat final tetap masuk milestone chat berikutnya.
      </article>
      <article class="rounded-3xl bg-ink-900 px-5 py-4 text-sm leading-6 text-white">
        Tekan tombol ping untuk mengirim event diagnostik ke channel user Anda melalui Redis
        pub/sub, lalu lihat event masuk pada timeline di bawah.
      </article>

      <div class="flex flex-wrap items-center gap-3">
        <Button variant="default" size="lg" onclick={pingRealtime}>
          Send Realtime Ping
        </Button>
        <p class="text-sm text-ink-600">
          Status: <span class="font-semibold text-ink-900">{$realtimeState.status}</span>
        </p>
      </div>

      <article class="rounded-3xl border border-ink-100 px-5 py-4 text-sm leading-6 text-ink-700">
        <p class="font-semibold text-ink-900">Subscribed Channels</p>
        <div class="mt-3 flex flex-wrap gap-2">
          {#each $realtimeState.channels as channel}
            <span class="rounded-full bg-canvas-100 px-3 py-1 text-xs font-semibold text-ink-700">
              {channel}
            </span>
          {/each}
        </div>
      </article>

      <article class="rounded-3xl border border-ink-100 px-5 py-4 text-sm leading-6 text-ink-700">
        <p class="font-semibold text-ink-900">Event Timeline</p>
        <div class="mt-4 space-y-3">
          {#if $realtimeState.events.length === 0}
            <p class="rounded-2xl bg-canvas-100 px-4 py-3 text-sm text-ink-600">
              Belum ada event realtime yang diterima pada sesi ini.
            </p>
          {:else}
            {#each $realtimeState.events as event}
              <div class="rounded-2xl bg-canvas-100 px-4 py-3">
                <p class="text-xs font-semibold uppercase tracking-[0.18em] text-brand-700">
                  {event.type}
                </p>
                <p class="mt-1 text-sm text-ink-900">{event.channel}</p>
                <p class="mt-1 text-xs text-ink-500">{event.created_at}</p>
              </div>
            {/each}
          {/if}
        </div>
      </article>
    </div>

    <aside class="rounded-3xl border border-dashed border-ink-100 px-4 py-4 text-sm leading-6 text-ink-700">
      <p class="font-semibold text-ink-900">Connection Detail</p>
      <p class="mt-3">Connection ID: {$realtimeState.connection_id ?? '-'}</p>
      <p class="mt-2">
        Last heartbeat:
        {$realtimeState.last_heartbeat_at ?? 'belum ada'}
      </p>
      <p class="mt-2">Reconnect attempt: {$realtimeState.reconnect_attempt}</p>
      <p class="mt-2">Last error: {$realtimeState.last_error ?? '-'}</p>
    </aside>
  </div>
</section>
