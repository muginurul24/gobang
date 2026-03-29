<svelte:head>
  <title>Chat | onixggr</title>
</svelte:head>

<section class="glass-panel overflow-hidden rounded-[2rem]">
  <header class="border-b border-ink-100 px-6 py-5">
    <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">Global Chat</p>
    <h2 class="mt-2 font-display text-3xl font-bold tracking-tight text-ink-900">
      Placeholder room untuk realtime global chat.
    </h2>
  </header>

  <div class="grid gap-4 p-6 lg:grid-cols-[1fr_18rem]">
    <div class="space-y-3">
      <article class="rounded-3xl bg-canvas-100 px-5 py-4 text-sm leading-6 text-ink-700">
        Belum ada message stream. Halaman ini hanya mengunci route, layout, dan pola visual untuk
        fitur realtime hari berikutnya.
      </article>
      <article class="rounded-3xl bg-ink-900 px-5 py-4 text-sm leading-6 text-white">
        Setelah websocket layer masuk, area ini akan menampilkan timeline global room dengan
        retensi 7 hari.
      </article>
    </div>

    <aside class="rounded-[1.5rem] border border-dashed border-ink-100 px-4 py-4 text-sm leading-6 text-ink-700">
      Sidebar ini disiapkan untuk status koneksi, badge unread, dan action moderation.
    </aside>
  </div>
</section>
