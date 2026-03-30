<script lang="ts">
  export let eyebrow = 'Empty State';
  export let title: string;
  export let body: string;
  export let actionHref = '';
  export let actionLabel = '';
</script>

<section class="rounded-3xl border border-dashed border-ink-200 bg-canvas-50 px-5 py-6 text-sm text-ink-700">
  <p class="text-xs font-semibold uppercase tracking-[0.24em] text-brand-700">{eyebrow}</p>
  <h3 class="mt-3 font-display text-2xl font-bold tracking-tight text-ink-900">{title}</h3>
  <p class="mt-3 max-w-2xl leading-6">{body}</p>

  {#if actionHref !== '' && actionLabel !== ''}
    <a
      class="mt-5 inline-flex rounded-full border border-ink-200 bg-white px-4 py-2 font-semibold text-ink-900 transition hover:border-accent-300 hover:text-accent-700"
      href={actionHref}
    >
      {actionLabel}
    </a>
  {/if}
</section>
